package store

import (
	"context"

	"github.com/google/uuid"
)

type ApplicationStatusStore interface {
	SetApplicationStatus(context.Context, string, uuid.UUID, bool, bool) (Application, error)
}

// Update only the requested status, preserving concurrent edits to other fields.
func (p *Postgres) SetApplicationStatus(ctx context.Context, id string, actor uuid.UUID, admin, active bool) (Application, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Application{}, err
	}
	defer tx.Rollback(ctx)
	app, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationColumns+` FROM oauth_clients WHERE id=$1 AND ($3 OR owner_id=$2) FOR UPDATE`, id, actor, admin))
	if err != nil {
		return Application{}, err
	}
	if app.IsActive != active {
		app, err = scanApplication(tx.QueryRow(ctx, `UPDATE oauth_clients SET is_active=$2,updated_at=NOW() WHERE id=$1 RETURNING `+applicationColumns, id, active))
		if err != nil {
			return Application{}, err
		}
		if err := invalidateApplication(ctx, tx, id); err != nil {
			return Application{}, err
		}
	}
	return app, tx.Commit(ctx)
}
