package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AccountStore interface {
	UpdateProfile(context.Context, uuid.UUID, *string) (User, error)
	ChangePassword(context.Context, uuid.UUID, string, string) error
}

func (p *Postgres) UpdateProfile(ctx context.Context, id uuid.UUID, name *string) (User, error) {
	user, err := scanUser(p.pool.QueryRow(ctx, `UPDATE users SET display_name=$2,updated_at=NOW() WHERE id=$1 AND is_active=true AND role IN ('ADMIN','SUPER_ADMIN')
	 RETURNING `+userColumns, id, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserForbidden
	}
	return user, err
}

func (p *Postgres) ChangePassword(ctx context.Context, id uuid.UUID, previousHash, newHash string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE users SET password_hash=$3,updated_at=NOW() WHERE id=$1 AND password_hash=$2 AND is_active=true`, id, previousHash, newHash)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	for _, query := range []string{
		`UPDATE user_sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`,
		`UPDATE oauth_tokens SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`,
		`UPDATE authorization_codes SET used_at=NOW() WHERE user_id=$1 AND used_at IS NULL`,
		`DELETE FROM external_login_flows WHERE user_id=$1`,
		`DELETE FROM oauth_consent_requests WHERE user_id=$1`,
	} {
		if _, err := tx.Exec(ctx, query, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
