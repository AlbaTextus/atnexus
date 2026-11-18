package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in integration tests only touch a uniquely named, temporary schema.
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ATHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ATHUB_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	schema := pgx.Identifier{fmt.Sprintf("athub_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migrations, err := filepath.Glob("../../prisma/migrations/*/migration.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 2 {
		t.Fatal("expected initial and volume migrations")
	}
	var legacyChapterID, legacyNovelID, legacyPlatformID int32
	for i, path := range migrations {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Prisma includes public schema creation; tests must not touch that schema.
		testMigration := strings.Replace(string(migration), `CREATE SCHEMA IF NOT EXISTS "public";`, "", 1)
		if _, err := pool.Exec(ctx, testMigration, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
		if i == 0 {
			// Seed the old schema so the next migration must preserve existing chapters.
			err := pool.QueryRow(ctx, `WITH p AS (
				INSERT INTO "Platform" ("name") VALUES ('legacy') RETURNING "id"
			), n AS (
				INSERT INTO "Novel" ("platformId", "title", "updatedAt")
				SELECT "id", 'Legacy', now() FROM p RETURNING "id", "platformId"
			), c AS (
				INSERT INTO "Chapter" ("novelId", "order", "title", "content", "updatedAt")
				SELECT "id", 1, 'Legacy chapter', 'Original body', '2026-10-28T00:00:00Z' FROM n
				RETURNING "id", "novelId"
			) SELECT c."id", c."novelId", n."platformId" FROM c JOIN n ON n."id" = c."novelId"`).Scan(&legacyChapterID, &legacyNovelID, &legacyPlatformID)
			if err != nil {
				t.Fatal(err)
			}
		}
		if filepath.Base(filepath.Dir(path)) == "20261102000000_user_api" {
			// The removal migration must work with populated account tables.
			_, err := pool.Exec(ctx, `INSERT INTO "ApiUser" ("id") VALUES ('retired');
				INSERT INTO "ApiKey" ("id", "userId", "hash", "cipher", "prefix")
				VALUES ('key', 'retired', 'hash', 'cipher', 'ath_test');
				INSERT INTO "ApiUsage" ("userId", "bucket", "category", "status")
				VALUES ('retired', now(), '/stats', 200)`, pgx.QueryExecModeSimpleProtocol)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var retiredTables int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name IN ('ApiUser', 'ApiKey', 'ApiUsage')`).Scan(&retiredTables); err != nil || retiredTables != 0 {
		t.Fatalf("retired tables remain: %d, %v", retiredTables, err)
	}
	store := NewPostgres(pool)
	if err := store.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if novel, err := store.GetNovel(ctx, legacyNovelID); err != nil || novel.TagIDs == nil || len(novel.TagIDs) != 0 {
		t.Fatalf("migration must leave existing novels without tags: %+v %v", novel, err)
	}
	legacy, err := store.GetChapter(ctx, legacyNovelID, 1)
	if err != nil || legacy.ID != legacyChapterID || legacy.VolumeID != nil || legacy.Title != "Legacy chapter" || legacy.Content != "Original body" || !legacy.UpdatedAt.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("migration changed existing chapter: %+v %v", legacy, err)
	}
	if err := store.DeleteNovel(ctx, legacyNovelID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM "Platform" WHERE "id" = $1`, legacyPlatformID); err != nil {
		t.Fatal(err)
	}
	var platformID, otherPlatformID, authorID int32
	if err := pool.QueryRow(ctx, `INSERT INTO "Platform" ("name") VALUES ('test') RETURNING "id"`).Scan(&platformID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO "Platform" ("name") VALUES ('other') RETURNING "id"`).Scan(&otherPlatformID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO "Author" ("platformId") VALUES ($1) RETURNING "id"`, platformID).Scan(&authorID); err != nil {
		t.Fatal(err)
	}
	input := NovelInput{PlatformID: platformID, AuthorID: &authorID, Title: "Book", Status: "ONGOING"}
	novel, err := store.CreateNovel(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if novel.ID <= 0 || novel.CreatedAt.IsZero() || novel.UpdatedAt.IsZero() {
		t.Fatalf("invalid novel: %+v", novel)
	}
	if got, err := store.GetNovel(ctx, novel.ID); err != nil || got.Title != input.Title {
		t.Fatalf("get: %+v %v", got, err)
	}
	if got, err := store.ListNovels(ctx, NovelFilter{PlatformID: platformID, AuthorID: authorID, Limit: 20}); err != nil || len(got) != 1 {
		t.Fatalf("list: %+v %v", got, err)
	}
	if got, err := store.ListChapters(ctx, novel.ID, 20, 0); err != nil || len(got) != 0 {
		t.Fatalf("empty chapters: %+v %v", got, err)
	}
	if _, err := store.ListChapters(ctx, novel.ID+100, 20, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing novel: %v", err)
	}
	wrongAuthor := input
	wrongAuthor.PlatformID = otherPlatformID
	if _, err := store.CreateNovel(ctx, wrongAuthor); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("cross-platform author: %v", err)
	}
	if _, err := store.UpdateNovel(ctx, novel.ID, wrongAuthor); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("cross-platform update: %v", err)
	}
	if _, err := store.UpdateNovel(ctx, novel.ID+100, input); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing update: %v", err)
	}
	input.Title, input.Status, input.AuthorID = "Updated", "COMPLETED", nil
	if got, err := store.UpdateNovel(ctx, novel.ID, input); err != nil || got.Title != "Updated" || got.AuthorID != nil {
		t.Fatalf("update: %+v %v", got, err)
	}
	input.Status = "ABANDONED"
	if got, err := store.UpdateNovel(ctx, novel.ID, input); err != nil || got.Status != input.Status {
		t.Fatalf("abandoned update: %+v %v", got, err)
	}
	if got, err := store.GetNovel(ctx, novel.ID); err != nil || got.Status != input.Status {
		t.Fatalf("abandoned read: %+v %v", got, err)
	}
	var volumeID, unusedVolumeID int32
	if err := pool.QueryRow(ctx, `INSERT INTO "Volume" ("novelId", "order", "title", "updatedAt")
		VALUES ($1, 1, 'Part 1', now()) RETURNING "id"`, novel.ID).Scan(&volumeID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO "Volume" ("novelId", "order", "title", "updatedAt")
		VALUES ($1, 2, 'Part 2', now()) RETURNING "id"`, novel.ID).Scan(&unusedVolumeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO "Volume" ("novelId", "order", "title", "updatedAt")
		VALUES ($1, 1, 'Duplicate', now())`, novel.ID); !errors.Is(databaseError(err), ErrConflict) {
		t.Fatalf("duplicate volume order: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO "Volume" ("novelId", "order", "title", "updatedAt")
		VALUES ($1, 1, 'Orphan', now())`, novel.ID+100); !errors.Is(databaseError(err), ErrInvalidReference) {
		t.Fatalf("orphan volume: %v", err)
	}
	chapter := ChapterInput{NovelID: novel.ID, Order: 1, Title: "Chapter", Content: "Original\n"}
	first, err := store.SaveChapter(ctx, chapter)
	if err != nil {
		t.Fatal(err)
	}
	if first.VolumeID != nil {
		t.Fatal("new ungrouped chapter has a volume")
	}
	chapter.Content = "Replaced\n"
	chapter.VolumeID = &volumeID
	second, err := store.SaveChapter(ctx, chapter)
	if err != nil || second.ID != first.ID || second.Content != chapter.Content || second.VolumeID == nil || *second.VolumeID != volumeID {
		t.Fatalf("upsert: %+v %v", second, err)
	}
	if got, err := store.GetChapter(ctx, novel.ID, 1); err != nil || got.Content != chapter.Content || got.VolumeID == nil || *got.VolumeID != volumeID {
		t.Fatalf("chapter: %+v %v", got, err)
	}
	otherNovel, err := store.CreateNovel(ctx, NovelInput{PlatformID: platformID, Title: "Other", Status: "ONGOING"})
	if err != nil {
		t.Fatal(err)
	}
	var otherVolumeID int32
	if err := pool.QueryRow(ctx, `INSERT INTO "Volume" ("novelId", "order", "title", "updatedAt")
		VALUES ($1, 1, 'Other part', now()) RETURNING "id"`, otherNovel.ID).Scan(&otherVolumeID); err != nil {
		t.Fatal(err)
	}
	invalidChapter := chapter
	invalidChapter.VolumeID = &otherVolumeID
	invalidChapter.Content = "Must not overwrite"
	if _, err := store.SaveChapter(ctx, invalidChapter); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("cross-novel volume update: %v", err)
	}
	invalidChapter.Order = 2
	if _, err := store.SaveChapter(ctx, invalidChapter); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("cross-novel volume insert: %v", err)
	}
	missingVolumeID := otherVolumeID + 100
	invalidChapter.VolumeID = &missingVolumeID
	if _, err := store.SaveChapter(ctx, invalidChapter); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("missing volume: %v", err)
	}
	if got, err := store.GetChapter(ctx, novel.ID, 1); err != nil || got.Content != chapter.Content || got.VolumeID == nil || *got.VolumeID != volumeID {
		t.Fatalf("rejected update changed chapter: %+v %v", got, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM "Volume" WHERE "id" = $1`, volumeID); !errors.Is(databaseError(err), ErrInvalidReference) {
		t.Fatalf("deleted nonempty volume: %v", err)
	}
	chapter.Order = 2
	chapter.VolumeID = nil
	if _, err := store.SaveChapter(ctx, chapter); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ListChapters(ctx, novel.ID, 1, 1); err != nil || len(got) != 1 || got[0].Order != 2 || got[0].VolumeID != nil {
		t.Fatalf("pagination: %+v %v", got, err)
	}
	if got, err := store.ListChapters(ctx, novel.ID, 1, 0); err != nil || len(got) != 1 || got[0].VolumeID == nil || *got[0].VolumeID != volumeID {
		t.Fatalf("grouped chapter list: %+v %v", got, err)
	}
	chapter.VolumeID = &volumeID
	if _, err := store.SaveChapter(ctx, chapter); err != nil {
		t.Fatal(err)
	}
	chapter.VolumeID = nil
	if got, err := store.SaveChapter(ctx, chapter); err != nil || got.VolumeID != nil {
		t.Fatalf("detach chapter: %+v %v", got, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM "Volume" WHERE "id" = $1`, unusedVolumeID); err != nil {
		t.Fatalf("delete empty volume: %v", err)
	}
	if got, err := store.ListChapters(ctx, novel.ID, 20, 100); err != nil || len(got) != 0 {
		t.Fatalf("empty page: %+v %v", got, err)
	}
	if err := store.DeleteChapter(ctx, novel.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteChapter(ctx, novel.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing chapter: %v", err)
	}
	if err := store.DeleteNovel(ctx, novel.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetChapter(ctx, novel.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cascade: %v", err)
	}
	var remainingVolumes int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM "Volume" WHERE "novelId" = $1`, novel.ID).Scan(&remainingVolumes); err != nil || remainingVolumes != 0 {
		t.Fatalf("volume cascade: %d %v", remainingVolumes, err)
	}
	if _, err := store.SaveChapter(ctx, chapter); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("orphan chapter: %v", err)
	}
	t.Run("tags", func(t *testing.T) { testTagIntegration(t, ctx, store) })
	t.Run("library pagination", func(t *testing.T) { testLibraryIntegration(t, ctx, store) })
	t.Run("upload authors", func(t *testing.T) { testUploadAuthors(t, ctx, store) })
	t.Run("illustration deletion", func(t *testing.T) { testIllustrationDeletion(t, ctx, store) })
	t.Run("illustration layout", func(t *testing.T) { testIllustrationLayout(t, ctx, store) })
	t.Run("word count", func(t *testing.T) { testWordCount(t, ctx, store) })
	t.Run("history", func(t *testing.T) { testHistoryIntegration(t, ctx, store) })
	t.Run("novel reference names", func(t *testing.T) { testNovelReferenceNames(t, ctx, store) })
}
