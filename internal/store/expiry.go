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

// PurgeATHubCallLog deletes ATHub call records made without a personal key
// that started before the given time: anonymous requests and browser-session
// traffic. They are an audit trail only. Calls made with a key are kept, since
// usage statistics and per-key totals are counted from them.
func (p *Postgres) PurgeATHubCallLog(ctx context.Context, before time.Time) (int64, error) {
	var removed int64
	for {
		tag, err := p.pool.Exec(ctx, `DELETE FROM athub_api_calls WHERE id IN (
			SELECT id FROM athub_api_calls WHERE key_id IS NULL AND started_at<$1 LIMIT $2)`, before, expiryPurgeBatch)
		if err != nil {
			return removed, err
		}
		removed += tag.RowsAffected()
		if tag.RowsAffected() < expiryPurgeBatch {
			return removed, nil
		}
	}
}

// RunExpiryPurge purges once at startup and then periodically until ctx ends.
// Keyless ATHub call records older than callLogRetention are purged as well;
// zero keeps them forever.
func (p *Postgres) RunExpiryPurge(ctx context.Context, logger *slog.Logger, callLogRetention time.Duration) {
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
		if callLogRetention > 0 {
			count, err := p.PurgeATHubCallLog(ctx, time.Now().Add(-callLogRetention))
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				logger.Error("purge ATHub call log", "error", err)
			}
			if count > 0 {
				logger.Info("purged ATHub call log", "rows", count)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
