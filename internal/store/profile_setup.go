package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProfileSetupStore interface {
	CompleteProfileSetup(context.Context, uuid.UUID, string) (User, error)
}

// The one-time update is atomic so a second request cannot rename the account.
func (p *Postgres) CompleteProfileSetup(ctx context.Context, userID uuid.UUID, name string) (User, error) {
	user, err := scanUser(p.pool.QueryRow(ctx, `UPDATE users SET display_name=$2,profile_setup_required=false,updated_at=NOW()
	 WHERE id=$1 AND is_active=true AND profile_setup_required=true RETURNING `+userColumns, userID, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrConflict
	}
	return user, err
}
