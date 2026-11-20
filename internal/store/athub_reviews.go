package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ATHubReview struct {
	ID           uuid.UUID       `json:"id"`
	UserID       *uuid.UUID      `json:"user_id"`
	Contributor  json.RawMessage `json:"contributor"`
	Operation    string          `json:"operation"`
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Query        string          `json:"query"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Status       string          `json:"status"`
	Reviewer     json.RawMessage `json:"reviewer,omitempty"`
	Note         string          `json:"note"`
	ResultStatus *int            `json:"result_status,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	ReviewedAt   *time.Time      `json:"reviewed_at"`
	// Baseline is what the content API found the change to alter when it was
	// submitted. The creator database keeps it with the queued review and hands
	// it to the executor on approval. No other read loads it, and it is never
	// served.
	Baseline json.RawMessage `json:"-"`
}
type ATHubReviewResult struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}
type ATHubReviewList struct {
	Items []ATHubReview `json:"items"`
	Total int64         `json:"total"`
}
type ATHubReviewExecutor func(context.Context, ATHubReview) (ATHubReviewResult, error)
type ATHubReviewStore interface {
	QueueATHubReview(context.Context, ATHubReview) (ATHubReview, error)
	ListATHubReviews(context.Context, uuid.UUID, bool, string, int) (ATHubReviewList, error)
	ATHubReview(context.Context, uuid.UUID, uuid.UUID, bool) (ATHubReview, error)
	DecideATHubReview(context.Context, uuid.UUID, uuid.UUID, string, string, ATHubReviewExecutor) (ATHubReview, error)
}

// ATHubPendingChange is a review awaiting a decision, as far as it concerns
// one novel: Baseline is the baseline queued with it, cut down to that novel.
// It names no submitter, because it is read by everyone who may edit the novel.
type ATHubPendingChange struct {
	ID        uuid.UUID
	CreatedAt time.Time
	Baseline  json.RawMessage
}
type ATHubPendingChangeList struct {
	Items []ATHubPendingChange
	Total int64
}

// ATHubPendingChangeStore tells which queued changes touch a novel, from the
// baselines the creator database keeps with them.
type ATHubPendingChangeStore interface {
	// PendingATHubChanges lists them oldest first, for an actor who may edit
	// novels, starting at the given offset.
	PendingATHubChanges(ctx context.Context, actor uuid.UUID, novelID int32, offset int) (ATHubPendingChangeList, error)
}

func reviewPermission(ctx context.Context, tx pgx.Tx, user uuid.UUID) (string, error) {
	var role string
	var active bool
	if err := tx.QueryRow(ctx, `SELECT role,is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, user).Scan(&role, &active); err != nil {
		return "", err
	}
	if !active || RoleLevel(role) < 3 {
		return "", ErrUserForbidden
	}
	policy, err := policyWith(ctx, tx, role, true)
	if err != nil {
		return "", err
	}
	if !policy.Rule("review", "", "").Allowed {
		return "", ErrUserForbidden
	}
	return role, nil
}
