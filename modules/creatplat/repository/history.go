package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NovelRevision struct {
	ID            uuid.UUID       `json:"id"`
	NovelID       int32           `json:"novelId"`
	UserID        *uuid.UUID      `json:"-"`
	Contributor   json.RawMessage `json:"contributor"`
	Changes       json.RawMessage `json:"changes"`
	CreatedAt     time.Time       `json:"createdAt"`
	StartedAt     time.Time       `json:"startedAt"`
	RevisionCount int64           `json:"revisionCount"`
	// Changes holds at most HistoryListChanges entries, without chapter body
	// diffs. ChangeCount and ChangeStats describe all of them.
	ChangeCount int64           `json:"changeCount"`
	ChangeStats json.RawMessage `json:"changeStats"`
}

// RevisionChanges is one page of the complete changes of a history entry.
type RevisionChanges struct {
	Changes json.RawMessage `json:"changes"`
	Total   int64           `json:"total"`
}

// HistoryListChanges bounds the changes listed per history entry. The rest is
// read page by page with NovelRevisionChanges.
const HistoryListChanges = 100

// Consecutive revisions by one account form one entry. Both the list and the
// detail query group the same way, so an entry's ID finds the same changes.
const groupedRevisions = `WITH visible AS (
		SELECT *,COALESCE(NULLIF(contributor->>'id',''),user_id::text,id::text) AS actor
		FROM novel_revisions WHERE novel_id=$1 AND status='completed'
	), boundaries AS (
		SELECT *,CASE WHEN actor=LAG(actor) OVER (ORDER BY created_at DESC,id DESC)
			THEN 0 ELSE 1 END AS boundary FROM visible
	), grouped AS (
		SELECT *,SUM(boundary) OVER (ORDER BY created_at DESC,id DESC ROWS UNBOUNDED PRECEDING) AS group_id
		FROM boundaries
	)`

// NovelRevisionChanges returns a page of the changes of the history entry that
// contains the given revision, in the order they were made.
func (p *History) NovelRevisionChanges(ctx context.Context, novelID int32, revision uuid.UUID, limit, offset int) (RevisionChanges, error) {
	var found bool
	var result RevisionChanges
	var changes []byte
	err := p.pool.QueryRow(ctx, groupedRevisions+`, items AS (
		SELECT c.value,ROW_NUMBER() OVER (ORDER BY g.created_at,g.id,c.ordinality) AS n
		FROM grouped g CROSS JOIN LATERAL jsonb_array_elements(g.changes) WITH ORDINALITY c(value,ordinality)
		WHERE g.group_id=(SELECT group_id FROM grouped WHERE id=$2)
	)
	SELECT EXISTS(SELECT 1 FROM grouped WHERE id=$2),(SELECT COUNT(*) FROM items),
		COALESCE((SELECT jsonb_agg(value ORDER BY n) FROM items WHERE n>$4 AND n<=$4::bigint+$3),'[]'::jsonb)`,
		novelID, revision, limit, offset).Scan(&found, &result.Total, &changes)
	if err != nil {
		return result, err
	}
	if !found {
		return result, p.notFound
	}
	result.Changes = changes
	return result, nil
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

type History struct {
	pool     *pgxpool.Pool
	notFound error
}

func NewHistory(pool *pgxpool.Pool, notFound error) *History {
	return &History{pool: pool, notFound: notFound}
}

func (p *History) CheckNovelHistorySchema(ctx context.Context) error {
	rows, err := p.pool.Query(ctx, `SELECT id,batch_id,novel_id,user_id,contributor,changes,status,created_at,completed_at FROM novel_revisions LIMIT 0`)
	if err == nil {
		rows.Close()
	}
	return err
}

func (p *History) PrepareNovelRevisions(ctx context.Context, batch uuid.UUID, revisions []NovelRevision) error {
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

// StaleRevisionBatches lists batches still pending although they were prepared
// before the given time, oldest first.
func (p *History) StaleRevisionBatches(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := p.pool.Query(ctx, `SELECT batch_id FROM novel_revisions WHERE status='pending' AND created_at<$1
		GROUP BY batch_id ORDER BY min(created_at) LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (p *History) FinishNovelRevisions(ctx context.Context, batch uuid.UUID, committed bool) error {
	status := "failed"
	if committed {
		status = "completed"
	}
	// A commit is authoritative: it also corrects a batch that was given up on
	// while its content transaction was still running.
	tag, err := p.pool.Exec(ctx, `UPDATE novel_revisions SET status=$2,completed_at=now()
		WHERE batch_id=$1 AND (status='pending' OR ($2='completed' AND status='failed'))`, batch, status)
	if err == nil && tag.RowsAffected() == 0 {
		return p.notFound
	}
	return err
}

func (p *History) NovelHistory(ctx context.Context, novelID int32, limit, offset int) (NovelHistory, error) {
	result := NovelHistory{Revisions: make([]NovelRevision, 0)}
	// Group consecutive submissions by stable account identity before pagination.
	// Keep the stored revisions intact; old imports benefit from the same view.
	// Missing identities are unique per revision and must never merge strangers.
	// An entry may hold thousands of changes with whole chapter diffs. The
	// list carries a bounded, diff-free sample plus counts of everything.
	rows, err := p.pool.Query(ctx, groupedRevisions+`, groups AS (
		SELECT group_id,(ARRAY_AGG(id ORDER BY created_at DESC,id DESC))[1] AS id,
			(ARRAY_AGG(contributor ORDER BY created_at DESC,id DESC))[1] AS contributor,
			MAX(created_at) AS created_at,MIN(created_at) AS started_at,COUNT(*) AS revision_count
		FROM grouped GROUP BY group_id
	), page AS (
		SELECT * FROM groups ORDER BY group_id LIMIT $2 OFFSET $3
	)
	SELECT p.id,p.contributor,COALESCE(c.changes,'[]'::jsonb),p.created_at,p.started_at,p.revision_count,t.total,
		COALESCE(c.change_count,0),COALESCE(s.stats,'[]'::jsonb)
	FROM (SELECT COUNT(*) AS total FROM groups) t LEFT JOIN page p ON true
	LEFT JOIN LATERAL (
		SELECT COUNT(*) AS change_count,jsonb_agg(x.value - 'bodyDiff' ORDER BY x.n) FILTER (WHERE x.n<=$4) AS changes
		FROM (SELECT c.value,ROW_NUMBER() OVER (ORDER BY g.created_at,g.id,c.ordinality) AS n
			FROM grouped g CROSS JOIN LATERAL jsonb_array_elements(g.changes) WITH ORDINALITY c(value,ordinality)
			WHERE g.group_id=p.group_id) x
	) c ON true
	LEFT JOIN LATERAL (
		SELECT jsonb_agg(jsonb_build_object('entity',k.entity,'action',k.action,'count',k.total) ORDER BY k.entity,k.action) AS stats
		FROM (SELECT c.value->>'entity' AS entity,c.value->>'action' AS action,COUNT(*) AS total
			FROM grouped g CROSS JOIN LATERAL jsonb_array_elements(g.changes) c(value)
			WHERE g.group_id=p.group_id GROUP BY 1,2) k
	) s ON true
	ORDER BY p.group_id`, novelID, limit, offset, HistoryListChanges)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var id *uuid.UUID
		var actor, changes, stats []byte
		var created, started *time.Time
		var count *int64
		var changeCount int64
		if err := rows.Scan(&id, &actor, &changes, &created, &started, &count, &result.Total, &changeCount, &stats); err != nil {
			return result, err
		}
		if id != nil {
			result.Revisions = append(result.Revisions, NovelRevision{
				ID: *id, NovelID: novelID, Contributor: actor, Changes: changes,
				CreatedAt: *created, StartedAt: *started, RevisionCount: *count,
				ChangeCount: changeCount, ChangeStats: stats,
			})
		}
	}
	return result, rows.Err()
}

func (p *History) NovelContributors(ctx context.Context, novelID int32) (json.RawMessage, error) {
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
