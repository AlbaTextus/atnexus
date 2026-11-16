package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BackgroundStore interface {
	UpdateBackground(context.Context, uuid.UUID, string, string) (User, error)
}

func (p *Postgres) UpdateBackground(ctx context.Context, id uuid.UUID, imageURL, originalURL string) (User, error) {
	user, err := scanUser(p.pool.QueryRow(ctx, `UPDATE users SET background_url=$2,background_original_url=$3,updated_at=NOW() WHERE id=$1 AND is_active=true RETURNING `+userColumns, id, imageURL, originalURL))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}
