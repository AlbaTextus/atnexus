package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Application is the management view of an OAuth client. Secrets are never serialized.
type Application struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	OwnerID           *uuid.UUID `json:"owner_id"`
	HomepageURL       string     `json:"homepage_url"`
	Description       string     `json:"description"`
	IconURL           string     `json:"icon_url"`
	MinimumRole       string     `json:"minimum_role"`
	RedirectURIs      []string   `json:"redirect_uris"`
	AllowedScopes     []string   `json:"allowed_scopes"`
	AllowedGrantTypes []string   `json:"allowed_grant_types"`
	IsActive          bool       `json:"is_active"`
	IsPublic          bool       `json:"is_public"`
	SecretAvailable   bool       `json:"secret_available"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ApplicationStore is separate from the OAuth runtime's minimal Store interface.
type ApplicationStore interface {
	ListApplications(context.Context, uuid.UUID, bool) ([]Application, error)
	ApplicationByID(context.Context, string, uuid.UUID, bool) (Application, error)
	CreateApplication(context.Context, Application, *string, *string) (Application, error)
	UpdateApplication(context.Context, Application, uuid.UUID, bool) (Application, error)
	DeleteApplication(context.Context, string, uuid.UUID, bool) error
	RotateApplicationSecret(context.Context, string, uuid.UUID, bool, string, string) error
	ApplicationSecret(context.Context, string, uuid.UUID, bool) (*string, error)
}

const applicationColumns = `id, name, owner_id, homepage_url, description, icon_url, minimum_role,
 redirect_uris, allowed_scopes, allowed_grant_types, is_active, (secret_hash IS NULL OR secret_hash = ''),
 (secret_ciphertext IS NOT NULL AND secret_ciphertext <> ''), created_at, updated_at`

func scanApplication(row pgx.Row) (Application, error) {
	var app Application
	err := row.Scan(&app.ID, &app.Name, &app.OwnerID, &app.HomepageURL, &app.Description, &app.IconURL, &app.MinimumRole,
		&app.RedirectURIs, &app.AllowedScopes, &app.AllowedGrantTypes, &app.IsActive, &app.IsPublic, &app.SecretAvailable, &app.CreatedAt, &app.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return app, ErrNotFound
	}
	return app, err
}

func (p *Postgres) ListApplications(ctx context.Context, actor uuid.UUID, admin bool) ([]Application, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+applicationColumns+` FROM oauth_clients WHERE ($2 OR owner_id=$1) ORDER BY created_at DESC, id`, actor, admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	apps := make([]Application, 0)
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (p *Postgres) ApplicationByID(ctx context.Context, id string, actor uuid.UUID, admin bool) (Application, error) {
	return scanApplication(p.pool.QueryRow(ctx, `SELECT `+applicationColumns+` FROM oauth_clients WHERE id=$1 AND ($3 OR owner_id=$2)`, id, actor, admin))
}

func (p *Postgres) CreateApplication(ctx context.Context, app Application, secretHash, encryptedSecret *string) (Application, error) {
	return scanApplication(p.pool.QueryRow(ctx, `INSERT INTO oauth_clients
	 (id,name,owner_id,homepage_url,description,icon_url,minimum_role,redirect_uris,allowed_scopes,allowed_grant_types,secret_hash,is_active,secret_ciphertext,created_at,updated_at)
	 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NOW(),NOW()) RETURNING `+applicationColumns,
		app.ID, app.Name, app.OwnerID, app.HomepageURL, app.Description, app.IconURL, app.MinimumRole, app.RedirectURIs, app.AllowedScopes, app.AllowedGrantTypes, secretHash, app.IsActive, encryptedSecret))
}

func (p *Postgres) UpdateApplication(ctx context.Context, app Application, actor uuid.UUID, admin bool) (Application, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Application{}, err
	}
	defer tx.Rollback(ctx)
	previous, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM oauth_clients WHERE id=$1 AND ($3 OR owner_id=$2) FOR UPDATE`, app.ID, actor, admin))
	if err != nil {
		return Application{}, err
	}
	updated, err := scanApplication(tx.QueryRow(ctx, `UPDATE oauth_clients SET name=$2,homepage_url=$3,description=$4,icon_url=$5,minimum_role=$6,redirect_uris=$7,is_active=$8,updated_at=NOW() WHERE id=$1 RETURNING `+applicationColumns,
		app.ID, app.Name, app.HomepageURL, app.Description, app.IconURL, app.MinimumRole, app.RedirectURIs, app.IsActive))
	if err != nil {
		return Application{}, err
	}
	if previous.IsActive != app.IsActive || previous.MinimumRole != app.MinimumRole || !equalStrings(previous.RedirectURIs, app.RedirectURIs) {
		if err := invalidateApplication(ctx, tx, app.ID); err != nil {
			return Application{}, err
		}
	}
	return updated, tx.Commit(ctx)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func invalidateApplication(ctx context.Context, tx pgx.Tx, id string) error {
	if _, err := tx.Exec(ctx, `UPDATE oauth_tokens SET revoked_at=NOW() WHERE client_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE authorization_codes SET used_at=NOW() WHERE client_id=$1 AND used_at IS NULL`, id)
	return err
}

// UpgradeClientSecretHash replaces a verified hash with an equivalent one for
// the same secret. A concurrent rotation changes the stored hash and wins.
func (p *Postgres) UpgradeClientSecretHash(ctx context.Context, id, current, next string) error {
	_, err := p.pool.Exec(ctx, `UPDATE oauth_clients SET secret_hash=$3 WHERE id=$1 AND secret_hash=$2`, id, current, next)
	return err
}

func (p *Postgres) RotateApplicationSecret(ctx context.Context, id string, actor uuid.UUID, admin bool, hash, encryptedSecret string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE oauth_clients SET secret_hash=$4,secret_ciphertext=$5,updated_at=NOW() WHERE id=$1 AND ($3 OR owner_id=$2) AND secret_hash IS NOT NULL AND secret_hash<>''`, id, actor, admin, hash, encryptedSecret)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := invalidateApplication(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ApplicationSecret(ctx context.Context, id string, actor uuid.UUID, admin bool) (*string, error) {
	var encrypted *string
	err := p.pool.QueryRow(ctx, `SELECT secret_ciphertext FROM oauth_clients WHERE id=$1 AND ($3 OR owner_id=$2)`, id, actor, admin).Scan(&encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return encrypted, err
}

func (p *Postgres) DeleteApplication(ctx context.Context, id string, actor uuid.UUID, admin bool) error {
	result, err := p.pool.Exec(ctx, `DELETE FROM oauth_clients WHERE id=$1 AND ($3 OR owner_id=$2)`, id, actor, admin)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23001") && pgErr.ConstraintName == "payment_orders_client_id_fkey" {
			return ErrApplicationHasPayments
		}
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
