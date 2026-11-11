package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type NovelRevision struct {
	ID          uuid.UUID       `json:"id"`
	NovelID     int32           `json:"novelId"`
	UserID      *uuid.UUID      `json:"-"`
	Contributor json.RawMessage `json:"contributor"`
	Changes     json.RawMessage `json:"changes"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type NovelHistory struct {
	Revisions []NovelRevision `json:"revisions"`
	Total     int64           `json:"total"`
}

type NovelHistoryStore interface {
	PrepareNovelRevisions(context.Context, uuid.UUID, []NovelRevision) error
	FinishNovelRevisions(context.Context, uuid.UUID, bool) error
	NovelHistory(context.Context, int32, int, int) (NovelHistory, error)
	NovelContributors(context.Context, int32) (json.RawMessage, error)
}

func (p *Postgres) CheckNovelHistorySchema(ctx context.Context) error {
	rows, err := p.pool.Query(ctx, `SELECT id,batch_id,novel_id,user_id,contributor,changes,status,created_at,completed_at FROM novel_revisions LIMIT 0`)
	if err == nil {
		rows.Close()
	}
	return err
}

func (p *Postgres) PrepareNovelRevisions(ctx context.Context, batch uuid.UUID, revisions []NovelRevision) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, revision := range revisions {
		if _, err := tx.Exec(ctx, `INSERT INTO novel_revisions
			(id,batch_id,novel_id,user_id,contributor,changes) VALUES($1,$2,$3,$4,$5,$6)`,
			revision.ID, batch, revision.NovelID, revision.UserID, revision.Contributor, revision.Changes); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) FinishNovelRevisions(ctx context.Context, batch uuid.UUID, committed bool) error {
	status := "failed"
	if committed {
		status = "completed"
	}
	tag, err := p.pool.Exec(ctx, `UPDATE novel_revisions SET status=$2,completed_at=now()
		WHERE batch_id=$1 AND status='pending'`, batch, status)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (p *Postgres) NovelHistory(ctx context.Context, novelID int32, limit, offset int) (NovelHistory, error) {
	result := NovelHistory{Revisions: make([]NovelRevision, 0)}
	// One statement keeps pagination and total in the same snapshot.
	rows, err := p.pool.Query(ctx, `WITH visible AS (
		SELECT * FROM novel_revisions WHERE novel_id=$1 AND status='completed'
	), page AS (SELECT * FROM visible ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3)
	SELECT p.id,p.novel_id,p.contributor,p.changes,p.created_at,t.total
	FROM (SELECT COUNT(*) AS total FROM visible) t LEFT JOIN page p ON true
	ORDER BY p.created_at DESC,p.id DESC`, novelID, limit, offset)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var id *uuid.UUID
		var novel *int32
		var actor, changes []byte
		var created *time.Time
		if err := rows.Scan(&id, &novel, &actor, &changes, &created, &result.Total); err != nil {
			return result, err
		}
		if id != nil {
			result.Revisions = append(result.Revisions, NovelRevision{ID: *id, NovelID: *novel, Contributor: actor, Changes: changes, CreatedAt: *created})
		}
	}
	return result, rows.Err()
}

func (p *Postgres) NovelContributors(ctx context.Context, novelID int32) (json.RawMessage, error) {
	// The creator must have an actual INSERT revision; the first later editor
	// of a legacy entry must never be presented as its creator.
	var result []byte
	err := p.pool.QueryRow(ctx, `WITH visible AS (
		SELECT * FROM novel_revisions WHERE novel_id=$1 AND status='completed'
		AND COALESCE(contributor->>'id','')<>''
	), people AS (
		SELECT DISTINCT ON (contributor->>'id') contributor,
			MIN(created_at) OVER(PARTITION BY contributor->>'id') AS first_at,
			COUNT(*) OVER(PARTITION BY contributor->>'id') AS edits
		FROM visible ORDER BY contributor->>'id',created_at DESC,id DESC
	), creator AS (
		SELECT contributor->>'id' AS id,created_at FROM visible v
		WHERE EXISTS(SELECT 1 FROM jsonb_array_elements(v.changes) c
			WHERE c->>'entity'='Novel' AND c->>'action'='INSERT')
		ORDER BY created_at,id LIMIT 1
	)
	SELECT jsonb_build_object(
		'creator',(SELECT p.contributor || jsonb_build_object('createdAt',c.created_at,'edits',p.edits)
			FROM people p JOIN creator c ON p.contributor->>'id'=c.id),
		'contributors',COALESCE((SELECT jsonb_agg(p.contributor ||
			jsonb_build_object('createdAt',p.first_at,'edits',p.edits) ORDER BY p.first_at,p.contributor->>'id')
			FROM people p WHERE NOT EXISTS(SELECT 1 FROM creator c WHERE c.id=p.contributor->>'id')),'[]'::jsonb))`,
		novelID).Scan(&result)
	return result, err
}
