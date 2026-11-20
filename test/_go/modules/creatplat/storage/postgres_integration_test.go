package storage

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/creatplat/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testSchema(t *testing.T, dsn, migrations string) string {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	name := "creator_split_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close(ctx)
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", name)
	parsed.RawQuery = query.Encode()
	conn, err := pgx.Connect(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	files, err := filepath.Glob(migrations)
	if err != nil || len(files) == 0 {
		t.Fatal("missing test migrations")
	}
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(sql), `CREATE SCHEMA IF NOT EXISTS "public";`, "")
		if _, err := conn.Exec(ctx, text, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(filepath.Dir(file)), err)
		}
	}
	return parsed.String()
}

func splitFixture(t *testing.T) (*store.Postgres, *Postgres, *pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	sourceURL, targetURL := os.Getenv("OAUTH_TEST_DATABASE_URL"), os.Getenv("CREATPLAT_TEST_DATABASE_URL")
	if sourceURL == "" || targetURL == "" {
		t.Skip("set OAUTH_TEST_DATABASE_URL and CREATPLAT_TEST_DATABASE_URL; tests use unique disposable schemas in separate databases")
	}
	sourceURL = testSchema(t, sourceURL, "../../../prisma/oauth/migrations/*/migration.sql")
	targetURL = testSchema(t, targetURL, "../../../prisma/creatplat/migrations/*/migration.sql")
	ctx := context.Background()
	source, err := pgxpool.New(ctx, sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(source.Close)
	target, err := pgxpool.New(ctx, targetURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(target.Close)
	accounts, err := store.NewPostgres(ctx, sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accounts.Close)
	creator, err := New(ctx, targetURL, accounts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(creator.Close)
	if err := creator.RequireSeparateDatabases(ctx, sourceURL); err != nil {
		t.Fatal(err)
	}
	if err := creator.RequireSeparateDatabases(ctx, targetURL); err == nil {
		t.Fatal("same database accepted as independent")
	}
	return accounts, creator, source, target
}

func TestSplitReviewLifecycleAndHistory(t *testing.T) {
	accounts, creator, source, target := splitFixture(t)
	ctx := context.Background()
	author, err := accounts.UpsertUser(ctx, store.UserSeed{Email: "split-author@example.test", PasswordHash: "unused", Role: "LEVEL_2"})
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := accounts.UpsertUser(ctx, store.UserSeed{Email: "split-reviewer@example.test", PasswordHash: "unused", Role: "ADMIN"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := accounts.CreateATHubKey(ctx, store.ATHubKey{ID: uuid.New(), UserID: author.ID, Name: "test", TokenHash: security.HashToken(uuid.NewString()), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	call := store.ATHubCall{ID: uuid.New(), RequestID: uuid.New(), UserID: &author.ID, KeyID: &key.ID, Method: "POST", Path: "/api/v2/tags", Authenticated: true}
	if err := accounts.BeginATHubCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	contributor, _ := json.Marshal(map[string]string{"id": author.ID.String(), "name": "投稿者"})
	item, err := creator.QueueATHubReview(ctx, store.ATHubReview{ID: call.ID, UserID: &author.ID, Contributor: contributor, Operation: "upload", Method: call.Method, Path: call.Path, Payload: json.RawMessage(`{"name":"待审标签"}`)})
	if err != nil {
		t.Fatal(err)
	}
	// Reviews and history are kept by the creator database alone: the account
	// database has no table that could hold either.
	var gone bool
	if err := source.QueryRow(ctx, `SELECT to_regclass('athub_reviews') IS NULL AND to_regclass('novel_revisions') IS NULL`).Scan(&gone); err != nil || !gone {
		t.Fatalf("the account database still has a review or history table: %v", err)
	}
	var queued int
	if err := target.QueryRow(ctx, `SELECT COUNT(*) FROM athub_reviews`).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("review was not kept in the creator database: %d %v", queued, err)
	}
	if _, err := creator.ListATHubReviews(ctx, author.ID, true, "", 0); !errors.Is(err, store.ErrUserForbidden) {
		t.Fatal("author could access all reviews", err)
	}
	// An image a pending review carries must not be deleted meanwhile.
	if mentioned, err := creator.PendingReviewMentions(ctx, "待审标签"); err != nil || !mentioned {
		t.Fatalf("pending review was not found by its payload: %v %v", mentioned, err)
	}
	if mentioned, err := creator.PendingReviewMentions(ctx, "别的标签"); err != nil || mentioned {
		t.Fatalf("a pending review mentions text it does not carry: %v %v", mentioned, err)
	}
	if _, err := creator.ATHubReview(ctx, reviewer.ID, item.ID, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("private review ownership bypassed", err)
	}
	var applied atomic.Int32
	execute := func(context.Context, store.ATHubReview) (store.ATHubReviewResult, error) {
		applied.Add(1)
		return store.ATHubReviewResult{Status: 201, Body: json.RawMessage(`{"id":1}`)}, nil
	}
	if _, err := source.Exec(ctx, `UPDATE users SET is_active=false WHERE id=$1`, author.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := creator.DecideATHubReview(ctx, reviewer.ID, item.ID, "approve", "", execute); !errors.Is(err, store.ErrATHubPermissionDenied) {
		t.Fatal("disabled submitter applied", err)
	}
	if applied.Load() != 0 {
		t.Fatal("rejected authorization executed content")
	}
	if _, err := source.Exec(ctx, `UPDATE users SET is_active=true WHERE id=$1`, author.ID); err != nil {
		t.Fatal(err)
	}
	result, err := creator.DecideATHubReview(ctx, reviewer.ID, item.ID, "approve", "", execute)
	if err != nil || result.Status != "approved" || applied.Load() != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := creator.DecideATHubReview(ctx, reviewer.ID, item.ID, "approve", "", execute); !errors.Is(err, store.ErrConflict) || applied.Load() != 1 {
		t.Fatal("repeat decision executed twice", err)
	}
	if mentioned, err := creator.PendingReviewMentions(ctx, "待审标签"); err != nil || mentioned {
		t.Fatalf("a decided review still counts as pending: %v %v", mentioned, err)
	}
	batch := uuid.New()
	if err := creator.PrepareNovelRevisions(ctx, batch, []repository.NovelRevision{{ID: uuid.New(), NovelID: 11, UserID: &author.ID, Contributor: contributor, Changes: json.RawMessage(`[]`)}}); err != nil {
		t.Fatal(err)
	}
	if err := creator.FinishNovelRevisions(ctx, batch, true); err != nil {
		t.Fatal(err)
	}
	var recorded int
	if err := target.QueryRow(ctx, `SELECT COUNT(*) FROM novel_revisions WHERE batch_id=$1 AND status='completed'`, batch).Scan(&recorded); err != nil || recorded != 1 {
		t.Fatalf("history was not kept in the creator database: %d %v", recorded, err)
	}
}
