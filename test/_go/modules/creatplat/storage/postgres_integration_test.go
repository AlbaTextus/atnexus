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

func splitFixture(t *testing.T) (*store.Postgres, *Postgres, *pgxpool.Pool, *pgxpool.Pool, string, string) {
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
	return accounts, creator, source, target, sourceURL, targetURL
}

func TestImportPreservesLegacyCreatorData(t *testing.T) {
	accounts, creator, source, target, sourceURL, _ := splitFixture(t)
	ctx := context.Background()
	user, err := accounts.CreateUser(ctx, "creator-import@example.test", "unused", nil)
	if err != nil {
		t.Fatal(err)
	}
	id, batch, revision := uuid.New(), uuid.New(), uuid.New()
	contributor, _ := json.Marshal(map[string]string{"id": user.ID.String(), "name": "真实迁移测试贡献者"})
	if _, err := source.Exec(ctx, `INSERT INTO athub_reviews(id,user_id,contributor,operation,method,path,payload,status,note) VALUES($1,$2,$3,'upload','POST','/api/v2/tags','{"name":"迁移测试"}','pending','保留审核备注')`, id, user.ID, contributor); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(ctx, `INSERT INTO novel_revisions(id,batch_id,novel_id,user_id,contributor,changes,status) VALUES($1,$2,7,$3,$4,'[{"entity":"Novel","action":"INSERT","after":{"title":"迁移测试"}}]','completed')`, revision, batch, user.ID, contributor); err != nil {
		t.Fatal(err)
	}
	if err := creator.CheckLegacyMigration(ctx, sourceURL); err == nil {
		t.Fatal("unimported legacy data accepted")
	}
	result, err := ImportLegacy(ctx, source, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reviews != 1 || result.History != 1 || result.AlreadyImported {
		t.Fatalf("result=%+v", result)
	}
	if err := creator.CheckLegacyMigration(ctx, sourceURL); err != nil {
		t.Fatal(err)
	}
	review, err := creator.ATHubReview(ctx, user.ID, id, false)
	if err != nil || review.ID != id || review.Note != "保留审核备注" || review.Status != "pending" {
		t.Fatalf("review=%+v err=%v", review, err)
	}
	history, err := creator.NovelHistory(ctx, 7, 20, 0)
	if err != nil || history.Total != 1 || history.Revisions[0].ID != revision {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	var count int
	if err := source.QueryRow(ctx, `SELECT COUNT(*) FROM athub_reviews`).Scan(&count); err != nil || count != 1 {
		t.Fatal("source was modified")
	}
	result, err = ImportLegacy(ctx, source, target)
	if err != nil || !result.AlreadyImported {
		t.Fatalf("repeat import=%+v err=%v", result, err)
	}
	if _, err := source.Exec(ctx, `UPDATE athub_reviews SET note='unexpected old writer' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := creator.CheckLegacyMigration(ctx, sourceURL); err == nil {
		t.Fatal("post-cutover source changes ignored")
	}
	if _, err := ImportLegacy(ctx, source, target); err == nil {
		t.Fatal("changed source overwrote destination")
	}
}

func TestSplitReviewLifecycleAndHistory(t *testing.T) {
	accounts, creator, source, target, sourceURL, _ := splitFixture(t)
	ctx := context.Background()
	if err := creator.CheckLegacyMigration(ctx, sourceURL); err != nil {
		t.Fatal(err)
	}
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
	call := store.ATHubCall{ID: uuid.New(), RequestID: uuid.New(), UserID: &author.ID, KeyID: &key.ID, Method: "POST", Path: "/api/v2/tags", Port: 8005, Authenticated: true}
	if err := accounts.BeginATHubCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	contributor, _ := json.Marshal(map[string]string{"id": author.ID.String(), "name": "投稿者"})
	item, err := creator.QueueATHubReview(ctx, store.ATHubReview{ID: call.ID, UserID: &author.ID, Contributor: contributor, Operation: "upload", Method: call.Method, Path: call.Path, Payload: json.RawMessage(`{"name":"待审标签"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var oldCount, newCount int
	if err := source.QueryRow(ctx, `SELECT COUNT(*) FROM athub_reviews`).Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if err := target.QueryRow(ctx, `SELECT COUNT(*) FROM athub_reviews`).Scan(&newCount); err != nil || oldCount != 0 || newCount != 1 {
		t.Fatal("review was not isolated in creator database")
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
	if err := source.QueryRow(ctx, `SELECT COUNT(*) FROM novel_revisions`).Scan(&oldCount); err != nil || oldCount != 0 {
		t.Fatal("history was written to account database")
	}
}

func TestImportFailureLeavesBothDatabasesIntact(t *testing.T) {
	_, _, source, target, _, _ := splitFixture(t)
	ctx := context.Background()
	if _, err := source.Exec(ctx, `INSERT INTO athub_reviews(id,contributor,operation,method,path,payload,note) VALUES($1,'{}','upload','POST','/api/v2/tags','{"large_number":9007199254740993}','reject_for_test')`, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Exec(ctx, `ALTER TABLE athub_reviews ADD CONSTRAINT migration_failure_test CHECK(note<>'reject_for_test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportLegacy(ctx, source, target); err == nil {
		t.Fatal("expected destination constraint failure")
	}
	var count int
	if err := target.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM athub_reviews)+(SELECT COUNT(*) FROM novel_revisions)+(SELECT COUNT(*) FROM creator_migration_state)`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed migration left partial destination data", err)
	}
	if err := source.QueryRow(ctx, `SELECT COUNT(*) FROM athub_reviews`).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed migration modified source", err)
	}
	if _, err := target.Exec(ctx, `ALTER TABLE athub_reviews DROP CONSTRAINT migration_failure_test`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportLegacy(ctx, source, target); err != nil {
		t.Fatal(err)
	}
	var number string
	if err := target.QueryRow(ctx, `SELECT payload->>'large_number' FROM athub_reviews`).Scan(&number); err != nil || number != "9007199254740993" {
		t.Fatalf("JSON precision lost: %q %v", number, err)
	}
}
