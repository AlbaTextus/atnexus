package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SessionByHashUncached is used by the shared module session authority so that
// a failed Redis invalidation cannot keep a revoked browser session authorized.
func (p *Postgres) SessionByHashUncached(ctx context.Context, tokenHash string) (Session, error) {
	var session Session
	err := p.pool.QueryRow(ctx, `SELECT s.id,s.user_id,s.expires_at,s.revoked_at
		FROM user_sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND u.is_active=true`, tokenHash).
		Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("authoritative session lookup: %w", err)
	}
	return session, nil
}
