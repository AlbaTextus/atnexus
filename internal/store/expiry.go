package store

import (
	"context"
	"log/slog"
	"time"
)

const (
	expiryPurgeInterval = 30 * time.Minute
	expiryPurgeBatch    = 5000
	// Rows are kept for a day past expiry so recent activity stays inspectable.
	expiryGrace = 24 * time.Hour
)

// Each statement removes one bounded batch of rows that can never be valid
// again; every lookup of these tables already rejects expired rows.
var expiryPurges = []struct{ table, statement string }{
	{"user_sessions", `DELETE FROM user_sessions WHERE id IN (SELECT id FROM user_sessions WHERE expires_at<$1 LIMIT $2)`},
	{"webauthn_ceremonies", `DELETE FROM webauthn_ceremonies WHERE id IN (SELECT id FROM webauthn_ceremonies WHERE expires_at<$1 LIMIT $2)`},
	{"authorization_codes", `DELETE FROM authorization_codes WHERE id IN (SELECT id FROM authorization_codes WHERE expires_at<$1 LIMIT $2)`},
	{"oauth_tokens", `DELETE FROM oauth_tokens WHERE id IN (SELECT id FROM oauth_tokens WHERE access_expires_at<$1 AND (refresh_expires_at IS NULL OR refresh_expires_at<$1) LIMIT $2)`},
}

// PurgeExpired deletes expired sessions, passkey ceremonies, authorization
// codes, tokens and rate-limit counters, which otherwise accumulate forever. It returns the number
// of rows removed per table.
func (p *Postgres) PurgeExpired(ctx context.Context) (map[string]int64, error) {
	removed := make(map[string]int64, len(expiryPurges))
	cutoff := time.Now().Add(-expiryGrace)
	for _, purge := range expiryPurges {
		for {
			tag, err := p.pool.Exec(ctx, purge.statement, cutoff, expiryPurgeBatch)
			if err != nil {
				return removed, err
			}
			removed[purge.table] += tag.RowsAffected()
			if tag.RowsAffected() < expiryPurgeBatch {
				break
			}
		}
	}
	// Rate-limit counters have no surrogate id and are few; one statement suffices.
	tag, err := p.pool.Exec(ctx, `DELETE FROM email_rate_limits WHERE expires_at<$1`, cutoff)
	if err != nil {
		return removed, err
	}
	removed["email_rate_limits"] = tag.RowsAffected()
	return removed, nil
}

// RunExpiryPurge purges once at startup and then periodically until ctx ends.
func (p *Postgres) RunExpiryPurge(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(expiryPurgeInterval)
	defer ticker.Stop()
	for {
		removed, err := p.PurgeExpired(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Error("purge expired credentials", "error", err)
		}
		for table, count := range removed {
			if count > 0 {
				logger.Info("purged expired credentials", "table", table, "rows", count)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
