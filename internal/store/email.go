package store

import (
	"context"
	"crypto/subtle"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrEmailCode      = errors.New("invalid or expired email code")
	ErrEmailRateLimit = errors.New("email rate limit exceeded")
)

type EmailChallenge struct {
	ID                                             uuid.UUID
	Purpose, Email, Actor, PreviousEmail, CodeHash string
}

type EmailRateLimit struct {
	Key    string
	Limit  int
	Window time.Duration
}

type EmailCompletion struct {
	EmailChallenge
	UserID       uuid.UUID
	PasswordHash string
	DisplayName  *string
	SessionHash  string
}

type EmailStore interface {
	ReserveEmailCode(context.Context, EmailChallenge, []EmailRateLimit) error
	ReadyEmailCode(context.Context, uuid.UUID) error
	DeleteEmailCode(context.Context, uuid.UUID) error
	CompleteEmailVerification(context.Context, EmailCompletion) (User, error)
}

func (p *Postgres) ReserveEmailCode(ctx context.Context, c EmailChallenge, limits []EmailRateLimit) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if c.Purpose == "register" {
		var registrationEnabled bool
		if err := tx.QueryRow(ctx, `SELECT registration_enabled FROM site_features WHERE id=1 FOR SHARE`).Scan(&registrationEnabled); err != nil {
			return err
		}
		if !registrationEnabled {
			return ErrRegistrationDisabled
		}
	}
	// Consistent lock order prevents concurrent sends from deadlocking.
	sort.Slice(limits, func(i, j int) bool { return limits[i].Key < limits[j].Key })
	for _, limit := range limits {
		result, err := tx.Exec(ctx, `INSERT INTO email_rate_limits (key,count,expires_at) VALUES ($1,1,NOW()+$2*INTERVAL '1 second')
		 ON CONFLICT (key) DO UPDATE SET count=CASE WHEN email_rate_limits.expires_at<=NOW() THEN 1 ELSE email_rate_limits.count+1 END,
		 expires_at=CASE WHEN email_rate_limits.expires_at<=NOW() THEN EXCLUDED.expires_at ELSE email_rate_limits.expires_at END
		 WHERE email_rate_limits.expires_at<=NOW() OR email_rate_limits.count<$3`, limit.Key, limit.Window.Seconds(), limit.Limit)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return ErrEmailRateLimit
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM email_challenges WHERE expires_at<NOW() OR (purpose=$1 AND email=$2 AND actor=$3)`, c.Purpose, c.Email, c.Actor); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM email_rate_limits WHERE expires_at<NOW()`); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO email_challenges (id,purpose,email,actor,previous_email,code_hash,expires_at) VALUES ($1,$2,$3,$4,$5,$6,NOW()+INTERVAL '5 minutes')`, c.ID, c.Purpose, c.Email, c.Actor, c.PreviousEmail, c.CodeHash)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ReadyEmailCode(ctx context.Context, id uuid.UUID) error {
	result, err := p.pool.Exec(ctx, `UPDATE email_challenges SET ready=true WHERE id=$1 AND expires_at>NOW()`, id)
	if err == nil && result.RowsAffected() == 0 {
		return ErrEmailCode
	}
	return err
}

func (p *Postgres) DeleteEmailCode(ctx context.Context, id uuid.UUID) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM email_challenges WHERE id=$1`, id)
	return err
}

// Code consumption and account mutation commit together. A bad attempt commits
// its counter, while transaction failures leave a valid code available to retry.
func (p *Postgres) CompleteEmailVerification(ctx context.Context, in EmailCompletion) (User, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var hash, previousEmail string
	var attempts int
	err = tx.QueryRow(ctx, `SELECT code_hash,previous_email,attempts FROM email_challenges
	 WHERE id=$1 AND purpose=$2 AND email=$3 AND actor=$4 AND ready=true AND expires_at>NOW() FOR UPDATE`, in.ID, in.Purpose, in.Email, in.Actor).Scan(&hash, &previousEmail, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrEmailCode
	}
	if err != nil {
		return User{}, err
	}
	if attempts >= 5 {
		return User{}, ErrEmailCode
	}
	if subtle.ConstantTimeCompare([]byte(hash), []byte(in.CodeHash)) != 1 {
		if _, err := tx.Exec(ctx, `UPDATE email_challenges SET attempts=attempts+1 WHERE id=$1`, in.ID); err != nil {
			return User{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return User{}, err
		}
		return User{}, ErrEmailCode
	}
	var user User
	switch in.Purpose {
	case "register":
		var registrationEnabled bool
		if err := tx.QueryRow(ctx, `SELECT registration_enabled FROM site_features WHERE id=1 FOR SHARE`).Scan(&registrationEnabled); err != nil {
			return User{}, err
		}
		if !registrationEnabled {
			return User{}, ErrRegistrationDisabled
		}
		user, err = scanUser(tx.QueryRow(ctx, `INSERT INTO users (id,email,password_hash,display_name,email_verified,role,is_active,created_at,updated_at,profile_setup_required)
		 VALUES ($1,$2,$3,$4,true,'USER',true,NOW(),NOW(),true) RETURNING `+userColumns, uuid.New(), in.Email, in.PasswordHash, in.DisplayName))
	case "account":
		user, err = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1 AND is_active=true FOR UPDATE`, in.UserID))
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrConflict
		}
		if err != nil {
			return User{}, err
		}
		if user.Email != previousEmail || user.PasswordHash != in.PasswordHash {
			return User{}, ErrConflict
		}
		var sessionOK bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_sessions WHERE token_hash=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>NOW())`, in.SessionHash, in.UserID).Scan(&sessionOK); err != nil {
			return User{}, err
		}
		if !sessionOK {
			return User{}, ErrConflict
		}
		user, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET email=$2,email_verified=true,updated_at=NOW() WHERE id=$1 RETURNING `+userColumns, in.UserID, in.Email))
		if err == nil && previousEmail != in.Email {
			for _, query := range []string{
				`UPDATE user_sessions SET revoked_at=NOW() WHERE user_id=$1 AND token_hash<>$2 AND revoked_at IS NULL`,
				`UPDATE oauth_tokens SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`,
				`UPDATE authorization_codes SET used_at=NOW() WHERE user_id=$1 AND used_at IS NULL`,
				`DELETE FROM external_login_flows WHERE user_id=$1`,
				`DELETE FROM oauth_consent_requests WHERE user_id=$1`,
			} {
				args := []any{in.UserID}
				if query == `UPDATE user_sessions SET revoked_at=NOW() WHERE user_id=$1 AND token_hash<>$2 AND revoked_at IS NULL` {
					args = append(args, in.SessionHash)
				}
				if _, err := tx.Exec(ctx, query, args...); err != nil {
					return User{}, err
				}
			}
		}
	default:
		return User{}, ErrEmailCode
	}
	if isUniqueViolation(err) {
		return User{}, ErrAlreadyExists
	}
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM email_challenges WHERE id=$1`, in.ID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	if in.Purpose == "account" && previousEmail != in.Email {
		p.invalidateUserSessionCache(ctx, in.UserID)
	}
	return user, nil
}
