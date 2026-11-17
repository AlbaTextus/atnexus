package storage

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Contributor struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type Change struct {
	Entity   string         `json:"entity"`
	Action   string         `json:"action"`
	Before   map[string]any `json:"before"`
	After    map[string]any `json:"after"`
	BodyDiff []BodyHunk     `json:"bodyDiff,omitempty"`
}

type RevisionDraft struct {
	NovelID     int32       `json:"novelId"`
	Contributor Contributor `json:"contributor"`
	Changes     []Change    `json:"changes"`
}

// The host owns persistence. A pending revision is durable before content commit
// and becomes visible only after completion.
type ChangeRecorder func(context.Context, []RevisionDraft) (func(context.Context, bool) error, error)

type contributorKey struct{}
type mutationKey struct{}

func WithContributor(ctx context.Context, contributor Contributor) context.Context {
	return context.WithValue(ctx, contributorKey{}, contributor)
}

type database interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type mutation struct {
	tx            pgx.Tx
	before        map[int32]map[string]map[string]any
	chapterBodies map[int32]string
}

func (p *Postgres) db(ctx context.Context) database {
	if m, ok := ctx.Value(mutationKey{}).(*mutation); ok {
		return m.tx
	}
	return p.pool
}

// Existing storage transactions become savepoints inside the request transaction.
func (p *Postgres) Mutate(ctx context.Context, fn func(context.Context) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	receipt, _ := ctx.Value(receiptKey{}).(*MutationReceipt)
	if receipt != nil {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,117))`, receipt.ID); err != nil {
			return err
		}
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT response FROM "MutationReceipt" WHERE id=$1::uuid`, receipt.ID).Scan(&raw)
		if err == nil {
			return json.Unmarshal(raw, receipt)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	m := &mutation{tx: tx, before: make(map[int32]map[string]map[string]any), chapterBodies: make(map[int32]string)}
	ctx = context.WithValue(ctx, mutationKey{}, m)
	if err := fn(ctx); err != nil {
		return err
	}
	drafts := make([]RevisionDraft, 0)
	actor, _ := ctx.Value(contributorKey{}).(Contributor)
	ids := make([]int, 0, len(m.before))
	for id := range m.before {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, value := range ids {
		id := int32(value)
		after, err := snapshot(ctx, tx, id)
		if err != nil {
			return err
		}
		changes := diffSnapshot(m.before[id], after)
		for i := range changes {
			change := &changes[i]
			if change.Entity != "Chapter" || change.Action != "UPDATE" || reflect.DeepEqual(change.Before["contentHash"], change.After["contentHash"]) {
				continue
			}
			chapterID := int32(change.Before["id"].(float64))
			oldBody, tracked := m.chapterBodies[chapterID]
			if !tracked {
				continue
			}
			var nextBody string
			if err := tx.QueryRow(ctx, `SELECT content FROM "Chapter" WHERE id=$1`, chapterID).Scan(&nextBody); err != nil {
				return err
			}
			change.BodyDiff = chapterBodyDiff(oldBody, nextBody)
		}
		if len(changes) > 0 {
			drafts = append(drafts, RevisionDraft{NovelID: id, Contributor: actor, Changes: changes})
		}
	}
	var finish func(context.Context, bool) error
	if len(drafts) > 0 && p.RecordChanges != nil {
		if finish, err = p.RecordChanges(ctx, drafts); err != nil {
			return err
		}
	}
	if receipt != nil {
		raw, encodeErr := json.Marshal(receipt)
		if encodeErr != nil {
			return encodeErr
		}
		if _, err := tx.Exec(ctx, `INSERT INTO "MutationReceipt"(id,response) VALUES($1::uuid,$2)`, receipt.ID, raw); err != nil {
			return err
		}
	}
	err = tx.Commit(ctx)
	if finish != nil {
		finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if finishErr := finish(finalCtx, err == nil); finishErr != nil {
			return errors.Join(err, finishErr)
		}
	}
	return databaseError(err)
}

// Lock the parent first so snapshots and concurrent changes cannot interleave.
func (p *Postgres) trackNovel(ctx context.Context, id int32, created bool) error {
	m, ok := ctx.Value(mutationKey{}).(*mutation)
	if !ok {
		return nil
	}
	if _, exists := m.before[id]; exists {
		return nil
	}
	if created {
		m.before[id] = map[string]map[string]any{}
		return nil
	}
	var locked int32
	if err := m.tx.QueryRow(ctx, `SELECT "id" FROM "Novel" WHERE "id"=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return databaseError(err)
	}
	before, err := snapshot(ctx, m.tx, id)
	if err == nil {
		m.before[id] = before
	}
	return err
}

func snapshot(ctx context.Context, q database, id int32) (map[string]map[string]any, error) {
	// Only metadata and stored chapter hashes/lengths; chapter bodies are never read.
	rows, err := q.Query(ctx, `SELECT 'Novel',n."id"::text,
		(to_jsonb(n)-'updatedAt'-'createdAt') || jsonb_build_object('tagIds',
			ARRAY(SELECT "tagId" FROM "NovelTag" WHERE "novelId"=n."id" ORDER BY "tagId"),
			'authorName',(SELECT name FROM "Author" WHERE id=n."authorId"),
			'platformName',(SELECT name FROM "Platform" WHERE id=n."platformId"),
			'tagNames',ARRAY(SELECT t.name FROM "Tag" t JOIN "NovelTag" nt ON nt."tagId"=t.id
				WHERE nt."novelId"=n.id ORDER BY t.id))
		FROM "Novel" n WHERE n."id"=$1
		UNION ALL SELECT 'Volume',v."id"::text,to_jsonb(v)-'updatedAt'-'createdAt'
		FROM "Volume" v WHERE "novelId"=$1
		UNION ALL SELECT 'Chapter',c."id"::text,
			jsonb_build_object('id',c."id",'novelId',c."novelId",'volumeId',c."volumeId",'order',c."order",
				'title',c."title",'contentHash',c."contentHash",'contentLength',c."contentLength")
		FROM "Chapter" c WHERE "novelId"=$1
		UNION ALL SELECT 'Illustration',i.id::text,to_jsonb(i)-'createdAt'
		FROM "Illustration" i WHERE "novelId"=$1
		UNION ALL SELECT 'IllustrationLayout',l."novelId"::text,to_jsonb(l)-'revision'
		FROM "IllustrationLayout" l WHERE "novelId"=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]map[string]any)
	for rows.Next() {
		var entity, itemID string
		var raw []byte
		if err := rows.Scan(&entity, &itemID, &raw); err != nil {
			return nil, err
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result[entity+":"+itemID] = value
	}
	return result, rows.Err()
}

func (p *Postgres) trackRelated(ctx context.Context, query string, id int32) error {
	if _, ok := ctx.Value(mutationKey{}).(*mutation); !ok {
		return nil
	}
	rows, err := p.db(ctx).Query(ctx, query, id)
	if err != nil {
		return err
	}
	ids := make([]int32, 0)
	for rows.Next() {
		var novelID int32
		if err := rows.Scan(&novelID); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, novelID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, novelID := range ids {
		if err := p.trackNovel(ctx, novelID, false); err != nil {
			return err
		}
	}
	return nil
}

func diffSnapshot(before, after map[string]map[string]any) []Change {
	keys := make(map[string]bool)
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	changes := make([]Change, 0)
	for _, key := range ordered {
		old, next := before[key], after[key]
		if reflect.DeepEqual(old, next) {
			continue
		}
		action := "UPDATE"
		if old == nil {
			action = "INSERT"
		}
		if next == nil {
			action = "DELETE"
		}
		entity, _, _ := strings.Cut(key, ":")
		changes = append(changes, Change{Entity: entity, Action: action, Before: old, After: next})
	}
	return changes
}
