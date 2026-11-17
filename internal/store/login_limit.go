package store

import (
	"context"
	"sort"
	"time"
)

// LoginLimit caps password logins counted under Key within one window.
type LoginLimit struct {
	Key   string
	Limit int
}

// LoginLimiter is implemented by stores that can count login attempts.
type LoginLimiter interface {
	ReserveLogin(context.Context, []LoginLimit, time.Duration) (time.Duration, error)
	ReleaseLogin(ctx context.Context, clear, uncount []string) error
}

// ReserveLogin counts one attempt under every key before the password is
// checked, so concurrent attempts cannot all pass a limit they would exceed
// together. When any counter is already at its limit nothing is counted and
// the time until that counter resets is returned. A counter's window starts at
// its first attempt and is not extended by later ones.
func (p *Postgres) ReserveLogin(ctx context.Context, limits []LoginLimit, window time.Duration) (time.Duration, error) {
	limits = append([]LoginLimit(nil), limits...)
	// Consistent lock order prevents concurrent attempts from deadlocking.
	sort.Slice(limits, func(i, j int) bool { return limits[i].Key < limits[j].Key })
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	for _, limit := range limits {
		// The condition is evaluated on the locked row, so each key is atomic.
		result, err := tx.Exec(ctx, `INSERT INTO email_rate_limits (key,count,expires_at) VALUES ($1,1,NOW()+$2*INTERVAL '1 second')
		 ON CONFLICT (key) DO UPDATE SET count=CASE WHEN email_rate_limits.expires_at<=NOW() THEN 1 ELSE email_rate_limits.count+1 END,
		 expires_at=CASE WHEN email_rate_limits.expires_at<=NOW() THEN EXCLUDED.expires_at ELSE email_rate_limits.expires_at END
		 WHERE email_rate_limits.expires_at<=NOW() OR email_rate_limits.count<$3`, limit.Key, window.Seconds(), limit.Limit)
		if err != nil {
			return 0, err
		}
		if result.RowsAffected() == 0 {
			if err = tx.Rollback(ctx); err != nil {
				return 0, err
			}
			return p.loginWait(ctx, limits)
		}
	}
	return 0, tx.Commit(ctx)
}

// loginWait reports how long until every counter that is at its limit resets.
func (p *Postgres) loginWait(ctx context.Context, limits []LoginLimit) (time.Duration, error) {
	wait := time.Second
	for _, limit := range limits {
		var seconds float64
		// key is CHAR(64); a parameter in this position is sent as that type,
		// so the lookup uses the primary key.
		err := p.pool.QueryRow(ctx, `SELECT COALESCE((SELECT EXTRACT(EPOCH FROM expires_at-NOW())::float8 FROM email_rate_limits
			WHERE key=$1 AND expires_at>NOW() AND count>=$2),0)`, limit.Key, limit.Limit).Scan(&seconds)
		if err != nil {
			return 0, err
		}
		wait = max(wait, time.Duration(seconds*float64(time.Second)))
	}
	return wait, nil
}

// ReleaseLogin settles a reserved attempt that turned out not to be a failed
// guess: counters under clear are forgotten and those under uncount give the
// attempt back. Each key is its own statement, so no two row locks are ever
// held together.
func (p *Postgres) ReleaseLogin(ctx context.Context, clear, uncount []string) error {
	for _, key := range clear {
		if _, err := p.pool.Exec(ctx, `DELETE FROM email_rate_limits WHERE key=$1`, key); err != nil {
			return err
		}
	}
	for _, key := range uncount {
		if _, err := p.pool.Exec(ctx, `UPDATE email_rate_limits SET count=GREATEST(count-1,0) WHERE key=$1 AND expires_at>NOW()`, key); err != nil {
			return err
		}
	}
	return nil
}
