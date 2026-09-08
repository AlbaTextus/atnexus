package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close()                         { p.pool.Close() }
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Resolve the selected avatar at read time so refreshed provider images and
// unlinked identities are reflected in every login and profile response.
const userColumns = `users.id,users.email,users.password_hash,users.display_name,users.role,users.is_active,users.created_at,
 users.avatar_source,CASE WHEN users.avatar_source='custom' THEN users.custom_avatar_url ELSE COALESCE((SELECT avatar_url FROM external_identities
 WHERE user_id=users.id AND avatar_url<>'' AND (users.avatar_source='auto' OR provider=users.avatar_source)
 ORDER BY created_at,id LIMIT 1),'') END,users.custom_avatar_url,users.numeric_id,users.bio`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role, &u.IsActive, &u.CreatedAt, &u.AvatarSource, &u.AvatarURL, &u.CustomAvatarURL, &u.NumericID, &u.Bio)
	return u, err
}

func (p *Postgres) CreateUser(ctx context.Context, email, passwordHash string, displayName *string) (User, error) {
	u, err := scanUser(p.pool.QueryRow(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, role, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'USER', true, NOW(), NOW())
		RETURNING `+userColumns, uuid.New(), email, passwordHash, displayName))
	if isUniqueViolation(err) {
		return User{}, ErrAlreadyExists
	}
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (p *Postgres) UserByEmail(ctx context.Context, email string) (User, error) {
	u, err := scanUser(p.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by email: %w", err)
	}
	return u, nil
}

func (p *Postgres) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	u, err := scanUser(p.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}
	return u, nil
}

// UpsertUser provisions an operator account for local/bootstrap environments.
// It deliberately never changes an existing user's password or role.
func (p *Postgres) UpsertUser(ctx context.Context, in UserSeed) (User, error) {
	u, err := scanUser(p.pool.QueryRow(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, role, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET updated_at = NOW()
		RETURNING `+userColumns, uuid.New(), in.Email, in.PasswordHash, in.DisplayName, in.Role))
	if err != nil {
		return User{}, fmt.Errorf("upsert user: %w", err)
	}
	return u, nil
}

func (p *Postgres) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO user_sessions (id, token_hash, user_id, expires_at, created_at) VALUES ($1, $2, $3, $4, NOW())`, uuid.New(), tokenHash, userID, expiresAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (p *Postgres) SessionByHash(ctx context.Context, tokenHash string) (Session, error) {
	var s Session
	err := p.pool.QueryRow(ctx, `SELECT s.id, s.user_id, s.expires_at, s.revoked_at FROM user_sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1 AND u.is_active = true`, tokenHash).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("find session: %w", err)
	}
	return s, nil
}

func (p *Postgres) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := p.pool.Exec(ctx, `UPDATE user_sessions SET revoked_at = COALESCE(revoked_at, NOW()) WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func scanPasskey(row pgx.Row) (PasskeyCredential, error) {
	var credential PasskeyCredential
	err := row.Scan(&credential.ID, &credential.UserID, &credential.CredentialID, &credential.PublicKey, &credential.AttestationType, &credential.Transports, &credential.SignCount, &credential.UserPresent, &credential.UserVerified, &credential.BackupEligible, &credential.BackupState, &credential.Name, &credential.CreatedAt, &credential.LastUsedAt)
	return credential, err
}

const passkeyColumns = `id, user_id, credential_id, public_key, attestation_type, transports, sign_count, user_present, user_verified, backup_eligible, backup_state, name, created_at, last_used_at`

func (p *Postgres) PasskeysByUserID(ctx context.Context, userID uuid.UUID) ([]PasskeyCredential, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+passkeyColumns+` FROM passkey_credentials WHERE user_id = $1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("find passkeys: %w", err)
	}
	defer rows.Close()
	credentials := make([]PasskeyCredential, 0)
	for rows.Next() {
		var credential PasskeyCredential
		if err := rows.Scan(&credential.ID, &credential.UserID, &credential.CredentialID, &credential.PublicKey, &credential.AttestationType, &credential.Transports, &credential.SignCount, &credential.UserPresent, &credential.UserVerified, &credential.BackupEligible, &credential.BackupState, &credential.Name, &credential.CreatedAt, &credential.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan passkey: %w", err)
		}
		credentials = append(credentials, credential)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate passkeys: %w", err)
	}
	return credentials, nil
}

func (p *Postgres) PasskeyByCredentialID(ctx context.Context, credentialID string) (PasskeyCredential, error) {
	credential, err := scanPasskey(p.pool.QueryRow(ctx, `SELECT `+passkeyColumns+` FROM passkey_credentials WHERE credential_id = $1`, credentialID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PasskeyCredential{}, ErrNotFound
	}
	if err != nil {
		return PasskeyCredential{}, fmt.Errorf("find passkey: %w", err)
	}
	return credential, nil
}

func (p *Postgres) CreatePasskey(ctx context.Context, credential PasskeyCredential) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO passkey_credentials (id, user_id, credential_id, public_key, attestation_type, transports, sign_count, user_present, user_verified, backup_eligible, backup_state, name, created_at, last_used_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, credential.ID, credential.UserID, credential.CredentialID, credential.PublicKey, credential.AttestationType, credential.Transports, credential.SignCount, credential.UserPresent, credential.UserVerified, credential.BackupEligible, credential.BackupState, credential.Name, credential.CreatedAt, credential.LastUsedAt)
	if isUniqueViolation(err) {
		return ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("create passkey: %w", err)
	}
	return nil
}

func (p *Postgres) UpdatePasskeyUsage(ctx context.Context, credentialID string, signCount int64, userPresent, userVerified, backupEligible, backupState bool) error {
	result, err := p.pool.Exec(ctx, `UPDATE passkey_credentials SET sign_count = $2, user_present = $3, user_verified = $4, backup_eligible = $5, backup_state = $6, last_used_at = NOW() WHERE credential_id = $1`, credentialID, signCount, userPresent, userVerified, backupEligible, backupState)
	if err != nil {
		return fmt.Errorf("update passkey usage: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) DeletePasskey(ctx context.Context, userID, id uuid.UUID) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM passkey_credentials WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete passkey: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) CreatePasskeyCeremony(ctx context.Context, tokenHash string, userID *uuid.UUID, kind string, sessionData []byte, expiresAt time.Time) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO webauthn_ceremonies (id, token_hash, user_id, kind, session_data, expires_at, created_at) VALUES ($1,$2,$3,$4,$5::jsonb,$6,NOW())`, uuid.New(), tokenHash, userID, kind, sessionData, expiresAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create webauthn ceremony: %w", err)
	}
	return nil
}

func (p *Postgres) ConsumePasskeyCeremony(ctx context.Context, tokenHash, kind string) (PasskeyCeremony, error) {
	var ceremony PasskeyCeremony
	err := p.pool.QueryRow(ctx, `UPDATE webauthn_ceremonies SET used_at = NOW() WHERE token_hash = $1 AND kind = $2 AND used_at IS NULL AND expires_at > NOW() RETURNING id, token_hash, user_id, kind, session_data, expires_at, used_at`, tokenHash, kind).Scan(&ceremony.ID, &ceremony.TokenHash, &ceremony.UserID, &ceremony.Kind, &ceremony.SessionData, &ceremony.ExpiresAt, &ceremony.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PasskeyCeremony{}, ErrNotFound
	}
	if err != nil {
		return PasskeyCeremony{}, fmt.Errorf("consume webauthn ceremony: %w", err)
	}
	return ceremony, nil
}

func (p *Postgres) ClientByID(ctx context.Context, id string) (Client, error) {
	var c Client
	err := p.pool.QueryRow(ctx, `SELECT id, name, secret_hash, redirect_uris, allowed_grant_types, allowed_scopes, access_token_ttl, refresh_token_ttl, is_active, minimum_role,homepage_url,description,icon_url,updated_at FROM oauth_clients WHERE id = $1`, id).Scan(&c.ID, &c.Name, &c.SecretHash, &c.RedirectURIs, &c.AllowedGrantTypes, &c.AllowedScopes, &c.AccessTokenTTL, &c.RefreshTokenTTL, &c.IsActive, &c.MinimumRole, &c.HomepageURL, &c.Description, &c.IconURL, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	if err != nil {
		return Client{}, fmt.Errorf("find oauth client: %w", err)
	}
	return c, nil
}

func (p *Postgres) UpsertClient(ctx context.Context, in ClientSeed) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO oauth_clients (id, name, secret_hash, redirect_uris, allowed_grant_types, allowed_scopes, access_token_ttl, refresh_token_ttl, secret_ciphertext, is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,true,NOW(),NOW())
		ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, secret_hash=EXCLUDED.secret_hash, secret_ciphertext=EXCLUDED.secret_ciphertext, redirect_uris=EXCLUDED.redirect_uris, allowed_grant_types=EXCLUDED.allowed_grant_types, allowed_scopes=EXCLUDED.allowed_scopes, access_token_ttl=EXCLUDED.access_token_ttl, refresh_token_ttl=EXCLUDED.refresh_token_ttl, updated_at=NOW()`, in.ID, in.Name, in.SecretHash, in.RedirectURIs, in.AllowedGrantTypes, in.AllowedScopes, in.AccessTokenTTL, in.RefreshTokenTTL, in.SecretCiphertext)
	if err != nil {
		return fmt.Errorf("upsert oauth client: %w", err)
	}
	return nil
}

func (p *Postgres) CreateAuthorizationCode(ctx context.Context, in NewAuthorizationCode) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO authorization_codes (id, code_hash, client_id, user_id, redirect_uri, scopes, code_challenge, code_challenge_method, expires_at, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())`, uuid.New(), in.CodeHash, in.ClientID, in.UserID, in.RedirectURI, in.Scopes, in.CodeChallenge, in.CodeChallengeMethod, in.ExpiresAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create authorization code: %w", err)
	}
	return nil
}

func (p *Postgres) ConsumeAuthorizationCode(ctx context.Context, codeHash string) (AuthorizationCode, error) {
	var c AuthorizationCode
	err := p.pool.QueryRow(ctx, `
		UPDATE authorization_codes SET used_at = NOW()
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		RETURNING id, code_hash, client_id, user_id, redirect_uri, scopes, code_challenge, code_challenge_method, expires_at, used_at`, codeHash).
		Scan(&c.ID, &c.CodeHash, &c.ClientID, &c.UserID, &c.RedirectURI, &c.Scopes, &c.CodeChallenge, &c.CodeChallengeMethod, &c.ExpiresAt, &c.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthorizationCode{}, ErrNotFound
	}
	if err != nil {
		return AuthorizationCode{}, fmt.Errorf("consume authorization code: %w", err)
	}
	return c, nil
}

func scanToken(row pgx.Row) (Token, error) {
	var t Token
	err := row.Scan(&t.ID, &t.AccessTokenHash, &t.RefreshTokenHash, &t.ClientID, &t.UserID, &t.Scopes, &t.AccessExpiresAt, &t.RefreshExpiresAt, &t.RevokedAt, &t.CreatedAt)
	return t, err
}

const tokenColumns = `id, access_token_hash, refresh_token_hash, client_id, user_id, scopes, access_expires_at, refresh_expires_at, revoked_at, created_at`

func (p *Postgres) CreateToken(ctx context.Context, in NewToken) (Token, error) {
	q := `INSERT INTO oauth_tokens (id, access_token_hash, refresh_token_hash, client_id, user_id, scopes, access_expires_at, refresh_expires_at, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW()) RETURNING ` + tokenColumns
	t, err := scanToken(p.pool.QueryRow(ctx, q, uuid.New(), in.AccessTokenHash, in.RefreshTokenHash, in.ClientID, in.UserID, in.Scopes, in.AccessExpiresAt, in.RefreshExpiresAt))
	if isUniqueViolation(err) {
		return Token{}, ErrConflict
	}
	if err != nil {
		return Token{}, fmt.Errorf("create token: %w", err)
	}
	return t, nil
}

func (p *Postgres) TokenByAccessHash(ctx context.Context, hash string) (Token, error) {
	t, err := scanToken(p.pool.QueryRow(ctx, `SELECT `+tokenColumns+` FROM oauth_tokens WHERE access_token_hash = $1 AND revoked_at IS NULL AND access_expires_at > NOW() AND EXISTS (SELECT 1 FROM oauth_clients c WHERE c.id=oauth_tokens.client_id AND c.is_active=true)`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, fmt.Errorf("find access token: %w", err)
	}
	return t, nil
}

func (p *Postgres) TokenByRefreshHash(ctx context.Context, hash string) (Token, error) {
	t, err := scanToken(p.pool.QueryRow(ctx, `SELECT `+tokenColumns+` FROM oauth_tokens WHERE refresh_token_hash = $1 AND revoked_at IS NULL AND refresh_expires_at > NOW() AND EXISTS (SELECT 1 FROM oauth_clients c WHERE c.id=oauth_tokens.client_id AND c.is_active=true)`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, fmt.Errorf("find refresh token: %w", err)
	}
	return t, nil
}

func (p *Postgres) RotateRefreshToken(ctx context.Context, oldHash string, in NewToken) (Token, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Token{}, fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer tx.Rollback(ctx)
	var oldID uuid.UUID
	err = tx.QueryRow(ctx, `UPDATE oauth_tokens SET revoked_at = NOW() WHERE refresh_token_hash = $1 AND revoked_at IS NULL AND refresh_expires_at > NOW() RETURNING id`, oldHash).Scan(&oldID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, fmt.Errorf("revoke old refresh token: %w", err)
	}
	q := `INSERT INTO oauth_tokens (id, access_token_hash, refresh_token_hash, client_id, user_id, scopes, access_expires_at, refresh_expires_at, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW()) RETURNING ` + tokenColumns
	var t Token
	err = tx.QueryRow(ctx, q, uuid.New(), in.AccessTokenHash, in.RefreshTokenHash, in.ClientID, in.UserID, in.Scopes, in.AccessExpiresAt, in.RefreshExpiresAt).Scan(&t.ID, &t.AccessTokenHash, &t.RefreshTokenHash, &t.ClientID, &t.UserID, &t.Scopes, &t.AccessExpiresAt, &t.RefreshExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		return Token{}, fmt.Errorf("create rotated token: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Token{}, fmt.Errorf("commit refresh rotation: %w", err)
	}
	return t, nil
}

func (p *Postgres) RevokeToken(ctx context.Context, tokenHash, clientID string) error {
	result, err := p.pool.Exec(ctx, `UPDATE oauth_tokens SET revoked_at = COALESCE(revoked_at, NOW()) WHERE (access_token_hash = $1 OR refresh_token_hash = $1) AND ($2 = '' OR client_id = $2)`, tokenHash, clientID)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
