package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const creatorMigrations = "../../../prisma/creatplat/migrations/*/migration.sql"

// openAuthority admits every submission, read and decision. These tests are
// about what the creator database keeps, not about account policy.
type openAuthority struct{}

func (openAuthority) WithCreatorSubmission(_ context.Context, _ store.ATHubReview, persist func() error) error {
	return persist()
}

func (openAuthority) WithCreatorReviewAccess(_ context.Context, _ uuid.UUID, read func() error) error {
	return read()
}

func (openAuthority) WithCreatorEditAccess(_ context.Context, _ uuid.UUID, read func() error) error {
	return read()
}

func (openAuthority) WithCreatorDecision(_ context.Context, _ uuid.UUID, _ store.ATHubReview, _ bool, apply func(json.RawMessage) error) error {
	return apply(json.RawMessage(`{"name":"审核人"}`))
}

func creatorOnly(t *testing.T) (*Postgres, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("CREATPLAT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CREATPLAT_TEST_DATABASE_URL; tests use a unique disposable schema")
	}
	ctx := context.Background()
	target := testSchema(t, dsn, creatorMigrations)
	pool, err := pgxpool.New(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	creator, err := New(ctx, target, openAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(creator.Close)
	return creator, pool
}

// decoded reads JSON without rounding its numbers.
func decoded(t *testing.T, raw []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	return value
}

// The baseline a submission is queued with reaches whoever applies it, on
// every attempt, as the same JSON value. Nothing else loads it.
func TestReviewBaselineReachesOnlyTheExecutor(t *testing.T) {
	creator, _ := creatorOnly(t)
	ctx := context.Background()
	author, reviewer := uuid.New(), uuid.New()
	// JSONB keeps the value, not the bytes: keys out of order, nesting, text
	// outside ASCII and an integer beyond 2^53 all have to come back.
	baseline := json.RawMessage(`{"v":1,"novels":{"17":[{"entity":"Novel","action":"UPDATE","before":{"title":"旧标题","id":17,"tagIds":[3,1,2]},"after":{"title":"新标题"}}]},"shared":[],"large":9007199254740993,"ratio":0.25}`)
	queued, err := creator.QueueATHubReview(ctx, store.ATHubReview{
		ID: uuid.New(), UserID: &author, Contributor: json.RawMessage(`{"name":"投稿者"}`),
		Operation: store.ATHubOperation("PUT", "/api/v2/novelinfo"), Method: "PUT", Path: "/api/v2/novelinfo", Query: "id=17",
		Payload: json.RawMessage(`{"title":"新标题"}`), Baseline: baseline,
	})
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != "pending" || queued.Baseline != nil {
		t.Fatalf("queued status=%q baseline=%s", queued.Status, queued.Baseline)
	}
	for _, all := range []bool{false, true} {
		actor := author
		if all {
			actor = reviewer
		}
		detail, err := creator.ATHubReview(ctx, actor, queued.ID, all)
		if err != nil || detail.ID != queued.ID || detail.Payload == nil || detail.Baseline != nil {
			t.Fatalf("all=%v detail=%+v err=%v", all, detail, err)
		}
		list, err := creator.ListATHubReviews(ctx, actor, all, "pending", 0)
		if err != nil || len(list.Items) != 1 || list.Items[0].ID != queued.ID || list.Items[0].Baseline != nil {
			t.Fatalf("all=%v list=%+v err=%v", all, list, err)
		}
	}
	// An outage leaves the review pending, and the retry is applied against the
	// same baseline. The reviews a decision returns carry none.
	var received []json.RawMessage
	if pending, err := creator.DecideATHubReview(ctx, reviewer, queued.ID, "approve", "", func(_ context.Context, r store.ATHubReview) (store.ATHubReviewResult, error) {
		received = append(received, r.Baseline)
		return store.ATHubReviewResult{Status: 503}, nil
	}); err == nil || pending.Status != "pending" || pending.Baseline != nil {
		t.Fatalf("an outage was not left pending: review=%+v err=%v", pending, err)
	}
	decided, err := creator.DecideATHubReview(ctx, reviewer, queued.ID, "approve", "", func(_ context.Context, r store.ATHubReview) (store.ATHubReviewResult, error) {
		received = append(received, r.Baseline)
		return store.ATHubReviewResult{Status: 200, Body: json.RawMessage(`{"id":17}`)}, nil
	})
	if err != nil || decided.Status != "approved" || decided.Baseline != nil {
		t.Fatalf("decided=%+v err=%v", decided, err)
	}
	if len(received) != 2 {
		t.Fatalf("executor ran %d times, want 2", len(received))
	}
	want := decoded(t, baseline)
	for attempt, got := range received {
		if !reflect.DeepEqual(decoded(t, got), want) {
			t.Fatalf("attempt %d was handed %s, want %s", attempt+1, got, baseline)
		}
	}
	if again, err := creator.DecideATHubReview(ctx, reviewer, queued.ID, "reject", "", nil); !errors.Is(err, store.ErrConflict) || again.Baseline != nil {
		t.Fatalf("a second decision: review=%+v err=%v", again, err)
	}
}

// A submission the content API reported no baseline for is stored without one
// and handed over without one, so it is applied in full.
func TestReviewWithoutBaselineHandsOverNone(t *testing.T) {
	creator, pool := creatorOnly(t)
	ctx := context.Background()
	author, reviewer := uuid.New(), uuid.New()
	for name, baseline := range map[string]json.RawMessage{"absent": nil, "empty": {}, "JSON null": json.RawMessage(`null`)} {
		queued, err := creator.QueueATHubReview(ctx, store.ATHubReview{
			ID: uuid.New(), UserID: &author, Contributor: json.RawMessage(`{"name":"投稿者"}`),
			Operation: store.ATHubOperation("POST", "/api/v2/tags"), Method: "POST", Path: "/api/v2/tags",
			Payload: json.RawMessage(`{"name":"待审标签"}`), Baseline: baseline,
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var stored bool
		if err := pool.QueryRow(ctx, `SELECT baseline IS NOT NULL FROM athub_reviews WHERE id=$1`, queued.ID).Scan(&stored); err != nil || stored {
			t.Fatalf("%s: a baseline was stored (%v), err=%v", name, stored, err)
		}
		handed := json.RawMessage(`"never handed over"`)
		decided, err := creator.DecideATHubReview(ctx, reviewer, queued.ID, "approve", "", func(_ context.Context, r store.ATHubReview) (store.ATHubReviewResult, error) {
			handed = r.Baseline
			return store.ATHubReviewResult{Status: 201, Body: json.RawMessage(`{"id":1}`)}, nil
		})
		if err != nil || decided.Status != "approved" || handed != nil {
			t.Fatalf("%s: decided=%+v handed=%s err=%v", name, decided, handed, err)
		}
	}
}

// A creator database that has not been migrated is refused at startup. Once it
// is migrated, the reviews it already held have no baseline and are handed over
// without one.
func TestReviewBaselineNeedsItsMigration(t *testing.T) {
	dsn := os.Getenv("CREATPLAT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CREATPLAT_TEST_DATABASE_URL; tests use a unique disposable schema")
	}
	ctx := context.Background()
	files, err := filepath.Glob(creatorMigrations)
	if err != nil || len(files) < 2 {
		t.Fatal("missing test migrations")
	}
	// The first migration alone is the database as it was before baselines.
	target := testSchema(t, dsn, files[0])
	pool, err := pgxpool.New(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	author, reviewer, id := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO athub_reviews(id,user_id,contributor,operation,method,path,payload) VALUES($1,$2,'{"name":"投稿者"}','upload','POST','/api/v2/tags','{"name":"升级前的标签"}')`, id, author); err != nil {
		t.Fatal(err)
	}
	if _, err := New(ctx, target, openAuthority{}); err == nil || !strings.Contains(err.Error(), "db:creatplat:deploy") || !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("unmigrated database: %v", err)
	}
	for _, file := range files[1:] {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(filepath.Dir(file)), err)
		}
	}
	creator, err := New(ctx, target, openAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(creator.Close)
	handed := json.RawMessage(`"never handed over"`)
	decided, err := creator.DecideATHubReview(ctx, reviewer, id, "approve", "", func(_ context.Context, r store.ATHubReview) (store.ATHubReviewResult, error) {
		handed = r.Baseline
		return store.ATHubReviewResult{Status: 201, Body: json.RawMessage(`{"id":1}`)}, nil
	})
	if err != nil || decided.Status != "approved" || handed != nil {
		t.Fatalf("decided=%+v handed=%s err=%v", decided, handed, err)
	}
}
