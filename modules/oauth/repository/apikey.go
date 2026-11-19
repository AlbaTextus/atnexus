// Package repository owns OAuth persistence while preserving existing schemas.
package repository

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var ErrATHubKeyLimit = errors.New("one current ATHub key per user")
var ErrATHubAdminDisabled = errors.New("ATHub access disabled by administrator")

type APIKeys struct {
	pool      *pgxpool.Pool
	notFound  error
}

func NewAPIKeys(pool *pgxpool.Pool, notFound error) *APIKeys {
	return &APIKeys{pool: pool, notFound: notFound}
}

type ATHubKey struct {
	AdminDisabled bool       `json:"admin_disabled"`
	ID            uuid.UUID  `json:"id"`
	UserID        uuid.UUID  `json:"user_id"`
	Name          string     `json:"name"`
	TokenHash     string     `json:"-"`
	Prefix        string     `json:"prefix"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	TotalCalls    int64      `json:"total_calls"`
}

type ATHubIdentity struct {
	KeyID  uuid.UUID
	UserID uuid.UUID
	Active bool
}

func (p *APIKeys) CreateATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, error) {
	result, created, err := p.manageATHubKey(ctx, key, false)
	if err == nil && !created {
		return result, ErrATHubKeyLimit
	}
	return result, err
}

func (p *APIKeys) EnsureATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, bool, error) {
	return p.manageATHubKey(ctx, key, false)
}

func (p *APIKeys) ResetATHubKey(ctx context.Context, key ATHubKey) (ATHubKey, error) {
	result, _, err := p.manageATHubKey(ctx, key, true)
	return result, err
}

func (p *APIKeys) manageATHubKey(ctx context.Context, key ATHubKey, reset bool) (ATHubKey, bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return key, false, err
	}
	defer tx.Rollback(ctx)
	var active, adminDisabled bool
	// Serialize provisioning and rotation for this user's single current slot.
	if err := tx.QueryRow(ctx, `SELECT is_active,athub_disabled FROM users WHERE id=$1 FOR UPDATE`, key.UserID).Scan(&active, &adminDisabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return key, false, p.notFound
		}
		return key, false, err
	}
	if !active {
		return key, false, p.notFound
	}
	if reset && adminDisabled {
		return key, false, ErrATHubAdminDisabled
	}
	if !reset {
		var current ATHubKey
		err := tx.QueryRow(ctx, `SELECT k.id,k.user_id,k.name,k.prefix,k.created_at,k.expires_at,k.revoked_at,k.last_used_at,
			(SELECT COUNT(*) FROM athub_api_calls c WHERE c.key_id=k.id)
			FROM athub_api_keys k WHERE k.user_id=$1 AND k.is_current`, key.UserID).Scan(
			&current.ID, &current.UserID, &current.Name, &current.Prefix, &current.CreatedAt, &current.ExpiresAt, &current.RevokedAt, &current.LastUsedAt, &current.TotalCalls)
		if err == nil {
			current.AdminDisabled = adminDisabled
			return current, false, tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return key, false, err
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE athub_api_keys SET is_current=false,revoked_at=COALESCE(revoked_at,now())
			WHERE user_id=$1 AND is_current`, key.UserID); err != nil {
			return key, false, err
		}
	}
	if adminDisabled {
		return key, false, ErrATHubAdminDisabled
	}
	key.ExpiresAt = nil
	err = tx.QueryRow(ctx, `INSERT INTO athub_api_keys(id,user_id,name,token_hash,prefix,expires_at,is_current)
		VALUES($1,$2,$3,$4,$5,NULL,true) RETURNING created_at`, key.ID, key.UserID, key.Name, key.TokenHash, key.Prefix).Scan(&key.CreatedAt)
	if err != nil {
		return key, false, err
	}
	return key, true, tx.Commit(ctx)
}

func (p *APIKeys) ListATHubKeys(ctx context.Context, userID uuid.UUID) ([]ATHubKey, error) {
	rows, err := p.pool.Query(ctx, `SELECT k.id,k.user_id,k.name,k.prefix,k.created_at,k.expires_at,k.revoked_at,k.last_used_at,u.athub_disabled,
		(SELECT COUNT(*) FROM athub_api_calls c WHERE c.key_id=k.id)
		FROM athub_api_keys k JOIN users u ON u.id=k.user_id WHERE k.user_id=$1 AND k.is_current ORDER BY k.created_at DESC,k.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]ATHubKey, 0)
	for rows.Next() {
		var key ATHubKey
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.CreatedAt, &key.ExpiresAt, &key.RevokedAt, &key.LastUsedAt, &key.AdminDisabled, &key.TotalCalls); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (p *APIKeys) RevokeATHubKey(ctx context.Context, userID, keyID uuid.UUID) error {
	tag, err := p.pool.Exec(ctx, `UPDATE athub_api_keys SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, keyID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return p.notFound
	}
	return nil
}

func (p *APIKeys) ResolveATHubKey(ctx context.Context, hash string) (ATHubIdentity, error) {
	var identity ATHubIdentity
	err := p.pool.QueryRow(ctx, `SELECT k.id,k.user_id,
		(u.is_active AND NOT u.athub_disabled AND k.is_current AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()))
		FROM athub_api_keys k JOIN users u ON u.id=k.user_id WHERE k.token_hash=$1`, hash).Scan(&identity.KeyID, &identity.UserID, &identity.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity, p.notFound
	}
	return identity, err
}
