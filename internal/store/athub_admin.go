package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ManagedATHubKey struct {
	Disabled      bool      `json:"admin_disabled"`
	AccountActive bool      `json:"account_active"`
	UpdatedAt     time.Time `json:"updated_at"`
	Key           *ATHubKey `json:"key"`
}

type ATHubAdminStore interface {
	ManagedATHubKey(context.Context, uuid.UUID, uuid.UUID) (ManagedATHubKey, error)
	SetManagedATHubKeyDisabled(context.Context, uuid.UUID, uuid.UUID, time.Time, bool) (ManagedATHubKey, error)
}

func managedATHubKey(ctx context.Context, tx pgx.Tx, id uuid.UUID) (ManagedATHubKey, error) {
	var result ManagedATHubKey
	if err := tx.QueryRow(ctx, `SELECT athub_disabled,is_active,updated_at FROM users WHERE id=$1`, id).Scan(&result.Disabled, &result.AccountActive, &result.UpdatedAt); err != nil {
		return result, err
	}
	var key ATHubKey
	err := tx.QueryRow(ctx, `SELECT k.id,k.user_id,k.name,k.prefix,k.created_at,k.expires_at,k.revoked_at,k.last_used_at,
		(SELECT COUNT(*) FROM athub_api_calls c WHERE c.key_id=k.id) FROM athub_api_keys k WHERE k.user_id=$1 AND k.is_current`, id).Scan(
		&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.CreatedAt, &key.ExpiresAt, &key.RevokedAt, &key.LastUsedAt, &key.TotalCalls)
	if err == nil {
		key.AdminDisabled = result.Disabled
		result.Key = &key
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	return result, nil
}

func (p *Postgres) ManagedATHubKey(ctx context.Context, actorID, id uuid.UUID) (ManagedATHubKey, error) {
	tx, actor, err := p.userManagementTx(ctx, actorID)
	if err != nil {
		return ManagedATHubKey{}, err
	}
	defer tx.Rollback(ctx)
	target, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedATHubKey{}, ErrNotFound
	}
	if err != nil {
		return ManagedATHubKey{}, err
	}
	if !canManageUser(actor, target.Role) {
		return ManagedATHubKey{}, ErrUserForbidden
	}
	result, err := managedATHubKey(ctx, tx, id)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (p *Postgres) SetManagedATHubKeyDisabled(ctx context.Context, actorID, id uuid.UUID, revision time.Time, disabled bool) (ManagedATHubKey, error) {
	tx, actor, err := p.userManagementTx(ctx, actorID)
	if err != nil {
		return ManagedATHubKey{}, err
	}
	defer tx.Rollback(ctx)
	target, err := managedTarget(ctx, tx, actor, id, revision)
	if err != nil {
		return ManagedATHubKey{}, err
	}
	if actorID == id || (!disabled && !target.IsActive) {
		return ManagedATHubKey{}, ErrUserForbidden
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET athub_disabled=$2,updated_at=clock_timestamp() WHERE id=$1`, id, disabled); err != nil {
		return ManagedATHubKey{}, err
	}
	if disabled {
		if _, err := tx.Exec(ctx, `UPDATE athub_api_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, id); err != nil {
			return ManagedATHubKey{}, err
		}
	}
	result, err := managedATHubKey(ctx, tx, id)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
