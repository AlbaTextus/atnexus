package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type MaintenanceState struct {
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MaintenanceStore interface {
	Maintenance(context.Context) (MaintenanceState, error)
	SetMaintenance(context.Context, bool, uuid.UUID) (MaintenanceState, error)
}

type MaintenanceIdentityStore interface {
	ExistingExternalUser(context.Context, string, string) (User, error)
}

// Look up an already-bound account without creating users or updating profiles.
func (p *Postgres) ExistingExternalUser(ctx context.Context, provider, subject string) (User, error) {
	return scanUser(p.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=(SELECT user_id FROM external_identities WHERE provider=$1 AND subject=$2)`, provider, subject))
}

func (p *Postgres) Maintenance(ctx context.Context) (MaintenanceState, error) {
	var state MaintenanceState
	err := p.pool.QueryRow(ctx, `SELECT enabled,updated_at FROM site_maintenance WHERE id=1`).Scan(&state.Enabled, &state.UpdatedAt)
	return state, err
}

func (p *Postgres) SetMaintenance(ctx context.Context, enabled bool, actor uuid.UUID) (MaintenanceState, error) {
	var state MaintenanceState
	err := p.pool.QueryRow(ctx, `UPDATE site_maintenance SET enabled=$1,updated_by=$2,updated_at=NOW() WHERE id=1 RETURNING enabled,updated_at`, enabled, actor).Scan(&state.Enabled, &state.UpdatedAt)
	return state, err
}
