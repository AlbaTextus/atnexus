package store

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BioStore interface {
	UpdateBio(context.Context, uuid.UUID, string) (User, error)
}

func (p *Postgres) UpdateBio(ctx context.Context, id uuid.UUID, bio string) (User, error) {
	user, err := scanUser(p.pool.QueryRow(ctx, `UPDATE users SET bio=$2,updated_at=NOW() WHERE id=$1 AND is_active=true RETURNING `+userColumns, id, bio))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}
