package store

import (
	"context"

	"github.com/google/uuid"
)

func (p *Postgres) CheckATHubCoverUpload(ctx context.Context, userID uuid.UUID) error {
	return p.checkATHubImageUpload(ctx, userID, ATHubCoverPath)
}

func (p *Postgres) CheckATHubIllustrationUpload(ctx context.Context, userID uuid.UUID) error {
	return p.checkATHubImageUpload(ctx, userID, "/api/v2/illustrations")
}

func (p *Postgres) checkATHubImageUpload(ctx context.Context, userID uuid.UUID, path string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var role string
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT role,is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, userID).Scan(&role, &enabled); err != nil {
		return err
	}
	if !enabled {
		return ErrATHubPermissionDenied
	}
	policy, err := policyWith(ctx, tx, role, true)
	if err != nil {
		return err
	}
	if !policy.Rule("upload", "POST", path).Allowed {
		return ErrATHubPermissionDenied
	}
	return tx.Commit(ctx)
}
