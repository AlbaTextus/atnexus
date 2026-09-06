package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ConsentRequest struct {
	TokenHash, ClientID, SessionHash string
	UserID                           uuid.UUID
	RequestData                      json.RawMessage
	ClientVersion, ExpiresAt         time.Time
}

type ConsentStore interface {
	CreateConsentRequest(context.Context, ConsentRequest) error
	ReadConsentRequest(context.Context, string, uuid.UUID, string) (ConsentRequest, error)
	CompleteConsentRequest(context.Context, ConsentRequest, *NewAuthorizationCode) error
}

var ErrConsentChanged = errors.New("consent application or account changed")

func (p *Postgres) CreateConsentRequest(ctx context.Context, in ConsentRequest) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM oauth_consent_requests WHERE expires_at<NOW()`); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO oauth_consent_requests(token_hash,user_id,client_id,session_hash,request_data,client_version,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, in.TokenHash, in.UserID, in.ClientID, in.SessionHash, in.RequestData, in.ClientVersion, in.ExpiresAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanConsent(row pgx.Row) (ConsentRequest, error) {
	var request ConsentRequest
	err := row.Scan(&request.TokenHash, &request.UserID, &request.ClientID, &request.SessionHash, &request.RequestData, &request.ClientVersion, &request.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return request, ErrNotFound
	}
	return request, err
}

const consentColumns = `token_hash,user_id,client_id,session_hash,request_data,client_version,expires_at`

func (p *Postgres) ReadConsentRequest(ctx context.Context, hash string, userID uuid.UUID, sessionHash string) (ConsentRequest, error) {
	return scanConsent(p.pool.QueryRow(ctx, `SELECT `+consentColumns+` FROM oauth_consent_requests WHERE token_hash=$1 AND user_id=$2 AND session_hash=$3 AND expires_at>NOW()`, hash, userID, sessionHash))
}

func (p *Postgres) CompleteConsentRequest(ctx context.Context, request ConsentRequest, code *NewAuthorizationCode) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var role, minimum string
	var active bool
	if err := tx.QueryRow(ctx, `SELECT role,is_active FROM users WHERE id=$1 FOR SHARE`, request.UserID).Scan(&role, &active); err != nil || !active {
		return ErrNotFound
	}
	var version time.Time
	if err := tx.QueryRow(ctx, `SELECT updated_at,is_active,minimum_role FROM oauth_clients WHERE id=$1 FOR SHARE`, request.ClientID).Scan(&version, &active, &minimum); err != nil || !active {
		return ErrConsentChanged
	}
	ranks := map[string]int{"USER": 1, "ADMIN": 2, "SUPER_ADMIN": 3}
	if minimum == "" {
		minimum = "USER"
	}
	if !version.Equal(request.ClientVersion) || ranks[minimum] == 0 || ranks[role] < ranks[minimum] {
		return ErrConsentChanged
	}
	var sessionID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM user_sessions WHERE token_hash=$1 AND user_id=$2 AND revoked_at IS NULL AND expires_at>clock_timestamp() FOR SHARE`, request.SessionHash, request.UserID).Scan(&sessionID); err != nil {
		return ErrNotFound
	}
	result, err := tx.Exec(ctx, `DELETE FROM oauth_consent_requests WHERE token_hash=$1 AND user_id=$2 AND session_hash=$3 AND expires_at>clock_timestamp()`, request.TokenHash, request.UserID, request.SessionHash)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if code != nil {
		if code.ClientID != request.ClientID || code.UserID != request.UserID {
			return ErrConsentChanged
		}
		_, err = tx.Exec(ctx, `INSERT INTO authorization_codes (id,code_hash,client_id,user_id,redirect_uri,scopes,code_challenge,code_challenge_method,expires_at,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())`, uuid.New(), code.CodeHash, code.ClientID, code.UserID, code.RedirectURI, code.Scopes, code.CodeChallenge, code.CodeChallengeMethod, code.ExpiresAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
