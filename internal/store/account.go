package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AvailableApplication contains only the information needed to visit a subsystem.
type AvailableApplication struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	HomepageURL string `json:"homepage_url"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
}

type AccountStore interface {
	AvailableApplications(context.Context, string) ([]AvailableApplication, error)
	UpdateProfile(context.Context, uuid.UUID, *string) (User, error)
	ChangePassword(context.Context, uuid.UUID, string, string) error
}

func (p *Postgres) AvailableApplications(ctx context.Context, role string) ([]AvailableApplication, error) {
	roles := []string{}
	switch role {
	case "SUPER_ADMIN":
		roles = []string{"USER", "ADMIN", "SUPER_ADMIN"}
	case "ADMIN":
		roles = []string{"USER", "ADMIN"}
	case "USER":
		roles = []string{"USER"}
	}
	rows, err := p.pool.Query(ctx, `SELECT id,name,homepage_url,description,icon_url FROM oauth_clients
	 WHERE is_active=true AND minimum_role=ANY($1::text[]) AND 'authorization_code'=ANY(allowed_grant_types)
	 ORDER BY name,id`, roles)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	apps := make([]AvailableApplication, 0)
	for rows.Next() {
		var app AvailableApplication
		if err := rows.Scan(&app.ID, &app.Name, &app.HomepageURL, &app.Description, &app.IconURL); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

func (p *Postgres) UpdateProfile(ctx context.Context, id uuid.UUID, name *string) (User, error) {
	user, err := scanUser(p.pool.QueryRow(ctx, `UPDATE users SET display_name=$2,updated_at=NOW() WHERE id=$1 AND is_active=true AND role IN ('ADMIN','SUPER_ADMIN')
	 RETURNING `+userColumns, id, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserForbidden
	}
	return user, err
}

func (p *Postgres) ChangePassword(ctx context.Context, id uuid.UUID, previousHash, newHash string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE users SET password_hash=$3,updated_at=NOW() WHERE id=$1 AND password_hash=$2 AND is_active=true`, id, previousHash, newHash)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrConflict
	}
	for _, query := range []string{
		`UPDATE user_sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`,
		`UPDATE oauth_tokens SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`,
		`UPDATE authorization_codes SET used_at=NOW() WHERE user_id=$1 AND used_at IS NULL`,
		`DELETE FROM external_login_flows WHERE user_id=$1`,
		`DELETE FROM oauth_consent_requests WHERE user_id=$1`,
	} {
		if _, err := tx.Exec(ctx, query, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	p.invalidateUserSessionCache(ctx, id)
	return nil
}
