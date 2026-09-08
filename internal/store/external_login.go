package store

import (
	"context"
	"errors"
	"time"

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrEmailLinkRequired     = errors.New("email requires explicit account linking")
	ErrVerifiedEmailRequired = errors.New("verified email required")
	ErrIdentityLinked        = errors.New("identity already linked")
	ErrPasswordRequired      = errors.New("password required before unlinking")
	ErrInactiveUser          = errors.New("account inactive")
)

type ExternalIdentity struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"-"`
	Provider      string    `json:"provider"`
	Subject       string    `json:"-"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"-"`
	AccountAlias  bool      `json:"-"`
	DisplayName   string    `json:"display_name"`
	AvatarURL     string    `json:"avatar_url"`
	CreatedAt     time.Time `json:"created_at"`
}

type ExternalLoginFlow struct {
	StateHash, BrowserHash, Provider, VerifierCiphertext, ReturnTo, SessionHash string
	UserID                                                                      *uuid.UUID
	ExpiresAt                                                                   time.Time
}

type ExternalLoginStore interface {
	CreateExternalLoginFlow(context.Context, ExternalLoginFlow) error
	ConsumeExternalLoginFlow(context.Context, string, string, string) (ExternalLoginFlow, error)
	ResolveExternalIdentity(context.Context, ExternalIdentity, *uuid.UUID) (User, error)
	ExternalIdentities(context.Context, uuid.UUID) ([]ExternalIdentity, error)
	UnlinkExternalIdentity(context.Context, uuid.UUID, string) error
}

func (p *Postgres) CreateExternalLoginFlow(ctx context.Context, flow ExternalLoginFlow) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM external_login_flows WHERE expires_at<NOW()`); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_login_flows (state_hash,browser_hash,provider,verifier_ciphertext,return_to,user_id,session_hash,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, flow.StateHash, flow.BrowserHash, flow.Provider, flow.VerifierCiphertext, flow.ReturnTo, flow.UserID, flow.SessionHash, flow.ExpiresAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ConsumeExternalLoginFlow(ctx context.Context, state, browser, provider string) (ExternalLoginFlow, error) {
	var flow ExternalLoginFlow
	err := p.pool.QueryRow(ctx, `DELETE FROM external_login_flows WHERE state_hash=$1 AND browser_hash=$2 AND provider=$3 AND expires_at>NOW() RETURNING state_hash,browser_hash,provider,verifier_ciphertext,return_to,user_id,session_hash,expires_at`, state, browser, provider).Scan(&flow.StateHash, &flow.BrowserHash, &flow.Provider, &flow.VerifierCiphertext, &flow.ReturnTo, &flow.UserID, &flow.SessionHash, &flow.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return flow, ErrNotFound
	}
	return flow, err
}

func (p *Postgres) ResolveExternalIdentity(ctx context.Context, identity ExternalIdentity, linkUser *uuid.UUID) (User, error) {
	identity.AvatarURL = security.SafeAvatarURL(identity.AvatarURL)
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize first-time logins for one upstream identity; database uniqueness remains authoritative.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, identity.Provider+":"+identity.Subject); err != nil {
		return User{}, err
	}
	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT user_id FROM external_identities WHERE provider=$1 AND subject=$2`, identity.Provider, identity.Subject).Scan(&existing)
	if err == nil {
		if linkUser != nil && *linkUser != existing {
			return User{}, ErrIdentityLinked
		}
		user, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1 FOR UPDATE`, existing))
		if err != nil {
			return User{}, err
		}
		if !user.IsActive {
			return User{}, ErrInactiveUser
		}
		if _, err := tx.Exec(ctx, `UPDATE external_identities SET email=$3,display_name=$4,avatar_url=$5 WHERE provider=$1 AND subject=$2`, identity.Provider, identity.Subject, identity.Email, identity.DisplayName, identity.AvatarURL); err != nil {
			return User{}, err
		}
		user, err = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, existing))
		if err != nil {
			return User{}, err
		}
		return user, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}
	var user User
	if linkUser != nil {
		user, err = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1 FOR UPDATE`, *linkUser))
		if err != nil {
			return User{}, err
		}
		if !user.IsActive {
			return User{}, ErrInactiveUser
		}
	} else {
		if (!identity.EmailVerified && !identity.AccountAlias) || identity.Email == "" {
			return User{}, ErrVerifiedEmailRequired
		}
		var occupied bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, identity.Email).Scan(&occupied); err != nil {
			return User{}, err
		}
		if occupied {
			return User{}, ErrEmailLinkRequired
		}
		name := identity.DisplayName
		user, err = scanUser(tx.QueryRow(ctx, `INSERT INTO users (id,email,password_hash,display_name,role,is_active,created_at,updated_at,email_verified,profile_setup_required) VALUES ($1,$2,'',$3,'USER',true,NOW(),NOW(),$4,true) RETURNING `+userColumns, uuid.New(), identity.Email, &name, identity.EmailVerified && !identity.AccountAlias))
		if isUniqueViolation(err) {
			return User{}, ErrEmailLinkRequired
		}
		if err != nil {
			return User{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO external_identities (id,user_id,provider,subject,email,display_name,avatar_url) VALUES ($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), user.ID, identity.Provider, identity.Subject, identity.Email, identity.DisplayName, identity.AvatarURL)
	if isUniqueViolation(err) {
		return User{}, ErrIdentityLinked
	}
	if err != nil {
		return User{}, err
	}
	user, err = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, user.ID))
	if err != nil {
		return User{}, err
	}
	return user, tx.Commit(ctx)
}

func (p *Postgres) ExternalIdentities(ctx context.Context, userID uuid.UUID) ([]ExternalIdentity, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,provider,email,display_name,avatar_url,created_at FROM external_identities WHERE user_id=$1 ORDER BY created_at,id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identities := make([]ExternalIdentity, 0)
	for rows.Next() {
		var identity ExternalIdentity
		if err := rows.Scan(&identity.ID, &identity.Provider, &identity.Email, &identity.DisplayName, &identity.AvatarURL, &identity.CreatedAt); err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, rows.Err()
}

func (p *Postgres) UnlinkExternalIdentity(ctx context.Context, userID uuid.UUID, provider string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var password string
	if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1 AND is_active=true FOR UPDATE`, userID).Scan(&password); err != nil {
		return err
	}
	if password == "" {
		return ErrPasswordRequired
	}
	result, err := tx.Exec(ctx, `DELETE FROM external_identities WHERE user_id=$1 AND provider=$2`, userID, provider)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET avatar_source='auto',updated_at=NOW() WHERE id=$1 AND avatar_source=$2`, userID, provider); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
