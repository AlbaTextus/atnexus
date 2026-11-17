package store

import (
	"context"
	"fmt"
)

// Rows are kept for a day past expiry so a just-expired credential still gets
// its precise error, then removed in bounded batches that hold locks briefly.
const purgeBatch = 5000

var purgeQueries = []struct{ name, table, expired string }{
	{"sessions", "user_sessions", `expires_at < now() - interval '1 day'`},
	{"authorization codes", "authorization_codes", `expires_at < now() - interval '1 day'`},
	// A token row is needed until both of its tokens have expired.
	{"tokens", "oauth_tokens", `access_expires_at < now() - interval '1 day' AND COALESCE(refresh_expires_at, access_expires_at) < now() - interval '1 day'`},
}

// PurgeExpired deletes credentials that can never authenticate again and
// returns how many rows were removed per kind. Nothing else reads these rows:
// every lookup already requires an unexpired one.
func (p *Postgres) PurgeExpired(ctx context.Context) (map[string]int64, error) {
	removed := make(map[string]int64, len(purgeQueries))
	for _, q := range purgeQueries {
		for {
			tag, err := p.pool.Exec(ctx, `DELETE FROM `+q.table+` WHERE ctid IN (SELECT ctid FROM `+q.table+` WHERE `+q.expired+` LIMIT $1)`, purgeBatch)
			if err != nil {
				return removed, fmt.Errorf("purge %s: %w", q.name, err)
			}
			removed[q.name] += tag.RowsAffected()
			if tag.RowsAffected() < purgeBatch {
				break
			}
		}
	}
	return removed, nil
}
