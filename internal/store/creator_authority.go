package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// WithCreatorSubmission keeps account admission and policy locks until the
// creator database has persisted the submission. The databases remain separate
// transactions; no account write is needed for this authorization callback.
func (p *Postgres) WithCreatorSubmission(ctx context.Context, review ATHubReview, persist func() error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var role string
	var active bool
	if err := tx.QueryRow(ctx, `SELECT role,is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, review.UserID).Scan(&role, &active); err != nil {
		return err
	}
	policy, err := policyWith(ctx, tx, role, true)
	if err != nil {
		return err
	}
	if !active || !policy.Rule(review.Operation, review.Method, review.Path).Allowed {
		return ErrATHubPermissionDenied
	}
	for _, edit := range ATHubUploadEdits(review.Path, review.Payload) {
		if !policy.Rule(edit.Operation, edit.Method, edit.Path).Allowed {
			return ErrATHubPermissionDenied
		}
	}
	var admitted bool
	if err := tx.QueryRow(ctx, `SELECT authenticated FROM athub_api_calls WHERE id=$1 AND user_id=$2 AND method=$3 AND path=$4`, review.ID, review.UserID, review.Method, review.Path).Scan(&admitted); err != nil {
		return err
	}
	if !admitted {
		return ErrATHubPermissionDenied
	}
	if err := persist(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) WithCreatorReviewAccess(ctx context.Context, actor uuid.UUID, read func() error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := reviewPermission(ctx, tx, actor); err != nil {
		return err
	}
	if err := read(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithCreatorEditAccess runs read for an account that may change an existing
// novel through the creator platform in at least one way. That is what lets it
// enter the edit mode of a novel's page, where it is shown what others have
// submitted for the same novel.
func (p *Postgres) WithCreatorEditAccess(ctx context.Context, actor uuid.UUID, read func() error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var role string
	var enabled bool
	if err := tx.QueryRow(ctx, `SELECT role,is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, actor).Scan(&role, &enabled); err != nil {
		return err
	}
	if !enabled {
		return ErrATHubPermissionDenied
	}
	policy, err := policyWith(ctx, tx, role, true)
	if err != nil {
		return err
	}
	allowed := false
	for _, edit := range ATHubNovelEdits() {
		allowed = allowed || policy.Rule(edit.Operation, edit.Method, edit.Path).Allowed
	}
	if !allowed {
		return ErrATHubPermissionDenied
	}
	if err := read(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithCreatorDecision preserves the original reviewer/author policy and ban
// checks while the creator module applies the approved content and persists its
// result. A creator/content receipt makes retries safe after partial commits.
func (p *Postgres) WithCreatorDecision(ctx context.Context, actor uuid.UUID, review ATHubReview, approve bool, apply func(json.RawMessage) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	role, err := reviewPermission(ctx, tx, actor)
	if err != nil {
		return err
	}
	if review.UserID != nil && *review.UserID == actor {
		return ErrUserForbidden
	}
	if approve {
		if review.UserID == nil {
			return ErrATHubPermissionDenied
		}
		reviewerPolicy, err := policyWith(ctx, tx, role, true)
		if err != nil {
			return err
		}
		if !reviewerPolicy.Rule(review.Operation, review.Method, review.Path).Allowed {
			return ErrUserForbidden
		}
		var authorRole string
		var active bool
		if err := tx.QueryRow(ctx, `SELECT role,is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, review.UserID).Scan(&authorRole, &active); err != nil {
			return err
		}
		authorPolicy, err := policyWith(ctx, tx, authorRole, true)
		if err != nil {
			return err
		}
		if !active || !authorPolicy.Rule(review.Operation, review.Method, review.Path).Allowed {
			return ErrATHubPermissionDenied
		}
		for _, edit := range ATHubUploadEdits(review.Path, review.Payload) {
			if !authorPolicy.Rule(edit.Operation, edit.Method, edit.Path).Allowed || !reviewerPolicy.Rule(edit.Operation, edit.Method, edit.Path).Allowed {
				return ErrATHubPermissionDenied
			}
		}
	}
	var reviewer []byte
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'name',COALESCE(display_name,'用户'),'role',role) FROM users WHERE id=$1`, actor).Scan(&reviewer); err != nil {
		return err
	}
	if err := apply(json.RawMessage(reviewer)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
