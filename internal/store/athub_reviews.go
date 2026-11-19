package store

import (
	"context"
	"encoding/json"
	"errors"
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

const reviewColumns = `id,user_id,contributor,operation,method,path,query,payload,status,reviewer,note,result_status,result,created_at,reviewed_at`
const reviewSummaryColumns = `id,user_id,contributor,operation,method,path,query,NULL::jsonb AS payload,status,reviewer,note,result_status,NULL::jsonb AS result,created_at,reviewed_at`

func scanReview(row interface{ Scan(...any) error }) (ATHubReview, error) {
	var r ATHubReview
	err := row.Scan(&r.ID, &r.UserID, &r.Contributor, &r.Operation, &r.Method, &r.Path, &r.Query, &r.Payload, &r.Status, &r.Reviewer, &r.Note, &r.ResultStatus, &r.Result, &r.CreatedAt, &r.ReviewedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return r, err
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
func (p *Postgres) QueueATHubReview(ctx context.Context, r ATHubReview) (ATHubReview, error) {
	if r.UserID == nil || !ATHubMutation(r.Operation) || ATHubOperation(r.Method, r.Path) != r.Operation || !json.Valid(r.Payload) {
		return r, ErrATHubPermissionDenied
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	var role string
	var active bool
	if err = tx.QueryRow(ctx, `SELECT role,is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, r.UserID).Scan(&role, &active); err != nil {
		return r, err
	}
	policy, err := policyWith(ctx, tx, role, true)
	if err != nil {
		return r, err
	}
	if !active || !policy.Rule(r.Operation, r.Method, r.Path).Allowed {
		return r, ErrATHubPermissionDenied
	}
	for _, edit := range ATHubUploadEdits(r.Path, r.Payload) {
		if !policy.Rule(edit.Operation, edit.Method, edit.Path).Allowed {
			return r, ErrATHubPermissionDenied
		}
	}
	// The source call already reserved quotas. Only admitted submissions can queue.
	var admitted bool
	if err = tx.QueryRow(ctx, `SELECT authenticated FROM athub_api_calls WHERE id=$1 AND user_id=$2 AND method=$3 AND path=$4`, r.ID, r.UserID, r.Method, r.Path).Scan(&admitted); err != nil {
		return r, err
	}
	if !admitted {
		return r, ErrATHubPermissionDenied
	}
	// The account database has no column for a baseline, so a review queued
	// here is applied in full when it is approved.
	result, err := scanReview(tx.QueryRow(ctx, `INSERT INTO athub_reviews(id,user_id,contributor,operation,method,path,query,payload)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+reviewColumns, r.ID, r.UserID, r.Contributor, r.Operation, r.Method, r.Path, r.Query, r.Payload))
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (p *Postgres) ListATHubReviews(ctx context.Context, actor uuid.UUID, all bool, status string, offset int) (ATHubReviewList, error) {
	result := ATHubReviewList{Items: []ATHubReview{}}
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if all {
		if _, err := reviewPermission(ctx, tx, actor); err != nil {
			return result, err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM athub_reviews WHERE ($1 OR user_id=$2) AND ($3='' OR status=$3)`, all, actor, status).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT `+reviewSummaryColumns+` FROM athub_reviews WHERE ($1 OR user_id=$2) AND ($3='' OR status=$3) ORDER BY created_at DESC,id DESC LIMIT 20 OFFSET $4`, all, actor, status, offset)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		item, err := scanReview(rows)
		if err != nil {
			rows.Close()
			return result, err
		}
		item.Payload = nil
		item.Result = nil
		result.Items = append(result.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (p *Postgres) ATHubReview(ctx context.Context, actor, id uuid.UUID, all bool) (ATHubReview, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return ATHubReview{}, err
	}
	defer tx.Rollback(ctx)
	if all {
		if _, err := reviewPermission(ctx, tx, actor); err != nil {
			return ATHubReview{}, err
		}
	}
	r, err := scanReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+` FROM athub_reviews WHERE id=$1 AND ($2 OR user_id=$3)`, id, all, actor))
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
func (p *Postgres) DecideATHubReview(ctx context.Context, actor, id uuid.UUID, decision, note string, execute ATHubReviewExecutor) (ATHubReview, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return ATHubReview{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize decisions with a fixed lock order; the content receipt also makes
	// a retry safe if content commits but this account transaction is interrupted.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(742190614)`); err != nil {
		return ATHubReview{}, err
	}
	role, err := reviewPermission(ctx, tx, actor)
	if err != nil {
		return ATHubReview{}, err
	}
	r, err := scanReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+` FROM athub_reviews WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return r, err
	}
	if r.Status != "pending" {
		return r, ErrConflict
	}
	if r.UserID != nil && *r.UserID == actor {
		return r, ErrUserForbidden
	}
	status := "rejected"
	if decision == "approve" {
		if r.UserID == nil || execute == nil {
			return r, ErrATHubPermissionDenied
		}
		// Review power alone never grants the ability to apply a deletion.
		reviewerPolicy, err := policyWith(ctx, tx, role, true)
		if err != nil {
			return r, err
		}
		if !reviewerPolicy.Rule(r.Operation, r.Method, r.Path).Allowed {
			return r, ErrUserForbidden
		}
		var authorRole string
		var active bool
		if err = tx.QueryRow(ctx, `SELECT role,is_active AND NOT athub_disabled FROM users WHERE id=$1 FOR SHARE`, r.UserID).Scan(&authorRole, &active); err != nil {
			return r, err
		}
		authorPolicy, err := policyWith(ctx, tx, authorRole, true)
		if err != nil {
			return r, err
		}
		if !active || !authorPolicy.Rule(r.Operation, r.Method, r.Path).Allowed {
			return r, ErrATHubPermissionDenied
		}
		for _, edit := range ATHubUploadEdits(r.Path, r.Payload) {
			if !authorPolicy.Rule(edit.Operation, edit.Method, edit.Path).Allowed || !reviewerPolicy.Rule(edit.Operation, edit.Method, edit.Path).Allowed {
				return r, ErrATHubPermissionDenied
			}
		}
		result, err := execute(ctx, r)
		if err != nil {
			return r, err
		}
		if result.Status >= 500 {
			return r, errors.New("content application unavailable")
		}
		status = "approved"
		if result.Status < 200 || result.Status >= 300 {
			status = "failed"
		}
		if !json.Valid(result.Body) {
			result.Body = json.RawMessage(`null`)
		}
		r.ResultStatus = &result.Status
		r.Result = result.Body
	} else if decision != "reject" {
		return r, ErrInvalidATHubKeyLimit
	}
	var reviewer []byte
	if err = tx.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'name',COALESCE(display_name,'用户'),'role',role) FROM users WHERE id=$1`, actor).Scan(&reviewer); err != nil {
		return r, err
	}
	r, err = scanReview(tx.QueryRow(ctx, `UPDATE athub_reviews SET status=$2,reviewer_id=$3,reviewer=$4,note=$5,result_status=$6,result=$7,reviewed_at=clock_timestamp() WHERE id=$1 RETURNING `+reviewColumns, id, status, actor, reviewer, note, r.ResultStatus, r.Result))
	if err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}
