package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrAvatarUnavailable = errors.New("avatar source unavailable")

type AvatarStore interface {
	UpdateAvatarSource(context.Context, uuid.UUID, string) (User, error)
}

type CustomAvatarStore interface {
	UpdateCustomAvatar(context.Context, uuid.UUID, string, string) (User, error)
}

func (p *Postgres) UpdateCustomAvatar(ctx context.Context, userID uuid.UUID, avatarURL, originalURL string) (User, error) {
	user, err := scanUser(p.pool.QueryRow(ctx, `UPDATE users SET custom_avatar_url=$2,avatar_original_url=$3,avatar_source='custom',updated_at=NOW() WHERE id=$1 AND is_active=true RETURNING `+userColumns, userID, avatarURL, originalURL))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInactiveUser
	}
	return user, err
}

func (p *Postgres) UpdateAvatarSource(ctx context.Context, userID uuid.UUID, source string) (User, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var active bool
	if err := tx.QueryRow(ctx, `SELECT is_active FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	if !active {
		return User{}, ErrInactiveUser
	}
	if source == "custom" {
		var available bool
		if err := tx.QueryRow(ctx, `SELECT custom_avatar_url<>'' FROM users WHERE id=$1`, userID).Scan(&available); err != nil {
			return User{}, err
		}
		if !available {
			return User{}, ErrAvatarUnavailable
		}
	} else if source != "auto" && source != "default" {
		var available bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM external_identities WHERE user_id=$1 AND provider=$2 AND avatar_url<>'')`, userID, source).Scan(&available); err != nil {
			return User{}, err
		}
		if !available {
			return User{}, ErrAvatarUnavailable
		}
	}
	user, err := scanUser(tx.QueryRow(ctx, `UPDATE users SET avatar_source=$2,updated_at=NOW() WHERE id=$1 RETURNING `+userColumns, userID, source))
	if err != nil {
		return User{}, err
	}
	return user, tx.Commit(ctx)
}
