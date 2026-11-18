package image

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Opt-in: uses a uniquely named temporary schema and temporary directories.
// A file is stored, found, and removed only under the address it was stored
// under, for the category and entity it belongs to.
func TestStoredFileLifecycleIntegration(t *testing.T) {
	dsn := os.Getenv("IMAGE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set IMAGE_TEST_DATABASE_URL to run the image database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	name := "image_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
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
	migrations, err := filepath.Glob("../../prisma/image/migrations/*/migration.sql")
	if err != nil || len(migrations) == 0 {
		conn.Close(ctx)
		t.Fatalf("migrations=%v err=%v", migrations, err)
	}
	for _, path := range migrations {
		sql, err := os.ReadFile(path)
		if err == nil {
			_, err = conn.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol)
		}
		if err != nil {
			conn.Close(ctx)
			t.Fatalf("migration %s: %v", filepath.Base(filepath.Dir(path)), err)
		}
	}
	conn.Close(ctx)

	files, scratch := t.TempDir(), t.TempDir()
	m, err := New(ctx, Config{DatabaseURL: parsed.String(), LocalRoot: files, TempRoot: scratch, PublicURL: "https://images.test",
		MaxUploadBytes: 1 << 20, MaxChunkBytes: 1 << 20, MaxChunks: 10, UploadTTL: time.Hour}, Dependencies{Sessions: noSession, Keys: keyStub{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)

	owner := uuid.NewString()
	cover, err := m.Save(ctx, owner, "book/covers", owner, "cover.jpg", "image/jpeg", []byte("cover"))
	if err != nil {
		t.Fatal(err)
	}
	stored := func(location string) bool {
		t.Helper()
		missing, err := m.Missing(ctx, location)
		if err != nil {
			t.Fatal(err)
		}
		return !missing
	}
	if !strings.HasPrefix(cover, "https://images.test/file/book/"+owner+"/covers/") || m.Address(cover) != cover || m.Address(cover+"?v=2") != cover {
		t.Fatalf("stored as %q", cover)
	}
	if !stored(cover) || !stored(cover+"?v=2") {
		t.Fatal("a stored file is reported missing")
	}
	if stored("https://images.test/file/book/"+owner+"/covers/never.jpg") || !stored("https://elsewhere.test/file/book/"+owner+"/covers/never.jpg") {
		t.Fatal("only this host's own files can be missing")
	}
	for _, attempt := range []struct{ category, entity, location string }{
		{"book/covers", "", cover + "?v=2"},
		{"book/covers", "", strings.Replace(cover, "/covers/", "%2Fcovers%2F", 1)},
		{"book/illustrations", "", cover},
		{"book/covers", uuid.NewString(), cover},
		{"user/avatars", owner, cover},
	} {
		if err := m.Remove(ctx, attempt.category, attempt.entity, attempt.location); err != nil {
			t.Fatalf("%+v: %v", attempt, err)
		}
		if !stored(cover) {
			t.Fatalf("%+v removed the file", attempt)
		}
	}
	if err := m.Remove(ctx, "book/covers", "", cover); err != nil || stored(cover) {
		t.Fatalf("the file was not removed: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(files, "book", owner, "covers")); err != nil || len(entries) != 0 {
		t.Fatalf("its bytes remain: %v %v", entries, err)
	}
	// A file that is already gone is not an error.
	if err := m.Remove(ctx, "book/covers", "", cover); err != nil {
		t.Fatal(err)
	}
	avatar, err := m.Save(ctx, owner, "user/avatars", owner, "avatar.png", "image/png", []byte("avatar"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(ctx, "user/avatars", owner, avatar); err != nil || stored(avatar) {
		t.Fatalf("the avatar was not removed: %v", err)
	}
}
