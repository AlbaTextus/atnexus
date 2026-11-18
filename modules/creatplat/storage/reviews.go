package storage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const reviewColumns = `id,user_id,contributor,operation,method,path,query,payload,status,reviewer,note,result_status,result,created_at,reviewed_at`
const reviewSummaryColumns = `id,user_id,contributor,operation,method,path,query,NULL::jsonb AS payload,status,reviewer,note,result_status,NULL::jsonb AS result,created_at,reviewed_at`

func scanReview(row interface{ Scan(...any) error }) (store.ATHubReview, error) {
	var r store.ATHubReview
	err := row.Scan(&r.ID, &r.UserID, &r.Contributor, &r.Operation, &r.Method, &r.Path, &r.Query, &r.Payload, &r.Status, &r.Reviewer, &r.Note, &r.ResultStatus, &r.Result, &r.CreatedAt, &r.ReviewedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = store.ErrNotFound
	}
	return r, err
}

func (p *Postgres) QueueATHubReview(ctx context.Context, r store.ATHubReview) (store.ATHubReview, error) {
	if r.UserID == nil || !store.ATHubMutation(r.Operation) || store.ATHubOperation(r.Method, r.Path) != r.Operation || !json.Valid(r.Payload) {
		return r, store.ErrATHubPermissionDenied
	}
	var result store.ATHubReview
	err := p.authority.WithCreatorSubmission(ctx, r, func() error {
		var err error
		result, err = scanReview(p.pool.QueryRow(ctx, `INSERT INTO athub_reviews(id,user_id,contributor,operation,method,path,query,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+reviewColumns, r.ID, r.UserID, r.Contributor, r.Operation, r.Method, r.Path, r.Query, r.Payload))
		return err
	})
	return result, err
}

func (p *Postgres) ListATHubReviews(ctx context.Context, actor uuid.UUID, all bool, status string, offset int) (store.ATHubReviewList, error) {
	result := store.ATHubReviewList{Items: []store.ATHubReview{}}
	read := func() error {
		tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM athub_reviews WHERE ($1 OR user_id=$2) AND ($3='' OR status=$3)`, all, actor, status).Scan(&result.Total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+reviewSummaryColumns+` FROM athub_reviews WHERE ($1 OR user_id=$2) AND ($3='' OR status=$3) ORDER BY created_at DESC,id DESC LIMIT 20 OFFSET $4`, all, actor, status, offset)
		if err != nil {
			return err
		}
		for rows.Next() {
			item, err := scanReview(rows)
			if err != nil {
				rows.Close()
				return err
			}
			item.Payload = nil
			item.Result = nil
			result.Items = append(result.Items, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var err error
	if all {
		err = p.authority.WithCreatorReviewAccess(ctx, actor, read)
	} else {
		err = read()
	}
	return result, err
}

func (p *Postgres) ATHubReview(ctx context.Context, actor, id uuid.UUID, all bool) (store.ATHubReview, error) {
	var result store.ATHubReview
	read := func() error {
		var err error
		result, err = scanReview(p.pool.QueryRow(ctx, `SELECT `+reviewColumns+` FROM athub_reviews WHERE id=$1 AND ($2 OR user_id=$3)`, id, all, actor))
		return err
	}
	var err error
	if all {
		err = p.authority.WithCreatorReviewAccess(ctx, actor, read)
	} else {
		err = read()
	}
	return result, err
}

func (p *Postgres) DecideATHubReview(ctx context.Context, actor, id uuid.UUID, decision, note string, execute store.ATHubReviewExecutor) (store.ATHubReview, error) {
	if decision != "approve" && decision != "reject" {
		return store.ATHubReview{}, store.ErrInvalidATHubKeyLimit
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return store.ATHubReview{}, err
	}
	defer tx.Rollback(ctx)
	// Preserve serialized decisions and content receipts across the database split.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(742190614)`); err != nil {
		return store.ATHubReview{}, err
	}
	r, err := scanReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+` FROM athub_reviews WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return r, err
	}
	if r.Status != "pending" {
		return r, store.ErrConflict
	}
	if r.UserID != nil && *r.UserID == actor {
		return r, store.ErrUserForbidden
	}
	err = p.authority.WithCreatorDecision(ctx, actor, r, decision == "approve", func(reviewer json.RawMessage) error {
		status := "rejected"
		if decision == "approve" {
			if r.UserID == nil || execute == nil {
				return store.ErrATHubPermissionDenied
			}
			result, err := execute(ctx, r)
			if err != nil {
				return err
			}
			if result.Status >= 500 {
				return errors.New("content application unavailable")
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
		}
		var err error
		r, err = scanReview(tx.QueryRow(ctx, `UPDATE athub_reviews SET status=$2,reviewer_id=$3,reviewer=$4,note=$5,result_status=$6,result=$7,reviewed_at=clock_timestamp() WHERE id=$1 RETURNING `+reviewColumns, id, status, actor, reviewer, note, r.ResultStatus, r.Result))
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	return r, err
}

// PendingReviewMentions reports whether a review still awaiting a decision
// carries the given text in its payload. An approval would write it back.
func (p *Postgres) PendingReviewMentions(ctx context.Context, text string) (bool, error) {
	var found bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM athub_reviews
		WHERE status='pending' AND position($1::text in payload::text)>0)`, text).Scan(&found)
	return found, err
}
