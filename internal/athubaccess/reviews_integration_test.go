package athubaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"albatextus/athub"
	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func moderationSchema(t *testing.T, dsn, kind, migrations string) string {
	t.Helper()
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := "review_e2e_" + kind + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := pgx.Identifier{schemaName}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
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
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	schemaURL := parsed.String()
	conn, err := pgx.Connect(ctx, schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	files, err := filepath.Glob(migrations)
	if err != nil || len(files) == 0 {
		t.Fatalf("migrations=%v err=%v", files, err)
	}
	for _, path := range files {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(sql), `CREATE SCHEMA IF NOT EXISTS "public";`, "")
		if _, err = conn.Exec(ctx, text, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	return schemaURL
}
func TestModeratedContentIntegration(t *testing.T) {
	dsn := os.Getenv("OAUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set OAUTH_TEST_DATABASE_URL for isolated moderation integration")
	}
	accountURL := moderationSchema(t, dsn, "account", "../../prisma/migrations/*/migration.sql")
	contentURL := moderationSchema(t, dsn, "content", "../../modules/athub/prisma/migrations/*/migration.sql")
	ctx := context.Background()
	p, err := store.NewPostgres(ctx, accountURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	access, err := New(p, ":8003", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	hub, err := athub.New(ctx, athub.Config{DatabaseURL: contentURL, WrapHandler: access.Wrap, RecordChanges: access.RecordChanges}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hub.Close)
	users := map[string]store.User{}
	tokens := map[string]string{}
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		name := role
		user, err := p.UpsertUser(ctx, store.UserSeed{Email: strings.ToLower(role) + "@example.test", PasswordHash: "unused-test-hash", DisplayName: &name, Role: role})
		if err != nil {
			t.Fatal(err)
		}
		token := "ath_" + strings.Repeat(strings.ToLower(role)[:1], 42) + fmt.Sprint(len(tokens))
		if _, err := p.CreateATHubKey(ctx, store.ATHubKey{ID: uuid.New(), UserID: user.ID, Name: "integration", Prefix: "ath_test", TokenHash: security.HashToken(token)}); err != nil {
			t.Fatal(err)
		}
		users[role] = user
		tokens[role] = token
	}
	call := func(role, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+tokens[role])
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		hub.Handler().ServeHTTP(response, request)
		return response
	}
	platform := call("SUPER_ADMIN", "POST", "/api/v2/platforms", `{"name":"review-platform"}`)
	if platform.Code != 201 {
		t.Fatalf("platform: %d %s", platform.Code, platform.Body)
	}
	var platformData struct {
		ID int32 `json:"id"`
	}
	_ = json.Unmarshal(platform.Body.Bytes(), &platformData)
	body := fmt.Sprintf(`{"platformId":%d,"title":"等待审核的小说","status":"ONGOING"}`, platformData.ID)
	denied := call("USER", "POST", "/api/v2/novelinfo", body)
	if denied.Code != 403 {
		t.Fatalf("LT1 write=%d %s", denied.Code, denied.Body)
	}
	queued := call("LEVEL_2", "POST", "/api/v2/novelinfo", body)
	if queued.Code != 202 {
		t.Fatalf("LT2 submission=%d %s", queued.Code, queued.Body)
	}
	var pending struct {
		ID uuid.UUID `json:"review_id"`
	}
	if err := json.Unmarshal(queued.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	before := call("USER", "GET", "/api/v2/novelinfo", "")
	if before.Code != 200 || strings.TrimSpace(before.Body.String()) != "[]" {
		t.Fatal("pending content became public", before.Body)
	}
	result, err := p.DecideATHubReview(ctx, users["ADMIN"].ID, pending.ID, "approve", "资料完整", access.ApplyReview)
	if err != nil || result.Status != "approved" {
		t.Fatalf("approval=%+v err=%v", result, err)
	}
	var novel struct {
		ID int32 `json:"id"`
	}
	if err := json.Unmarshal(result.Result, &novel); err != nil {
		t.Fatal(err)
	}
	visible := call("USER", "GET", fmt.Sprintf("/api/v2/novelinfo?id=%d", novel.ID), "")
	if visible.Code != 200 || !strings.Contains(visible.Body.String(), "等待审核的小说") {
		t.Fatal("approved content not visible", visible.Body)
	}
	history, err := p.NovelHistory(ctx, novel.ID, 20, 0)
	if err != nil || history.Total != 1 || !strings.Contains(string(history.Revisions[0].Contributor), users["LEVEL_2"].ID.String()) {
		t.Fatalf("original contributor lost history=%+v err=%v", history, err)
	}
	editBody := fmt.Sprintf(`{"platformId":%d,"title":"不应生效的修改","status":"ONGOING"}`, platformData.ID)
	edit := call("LEVEL_2", "PUT", fmt.Sprintf("/api/v2/novelinfo?id=%d", novel.ID), editBody)
	if edit.Code != 202 {
		t.Fatalf("edit=%d %s", edit.Code, edit.Body)
	}
	_ = json.Unmarshal(edit.Body.Bytes(), &pending)
	if _, err := p.DecideATHubReview(ctx, users["ADMIN"].ID, pending.ID, "reject", "资料不完整", access.ApplyReview); err != nil {
		t.Fatal(err)
	}
	visible = call("USER", "GET", fmt.Sprintf("/api/v2/novelinfo?id=%d", novel.ID), "")
	if strings.Contains(visible.Body.String(), "不应生效") {
		t.Fatal("rejected mutation applied")
	}
	deletion := call("ADMIN", "DELETE", fmt.Sprintf("/api/v2/novelinfo?id=%d", novel.ID), "")
	if deletion.Code != 403 {
		t.Fatalf("LT3 deleted content: %d", deletion.Code)
	}
	deletion = call("SUPER_ADMIN", "DELETE", fmt.Sprintf("/api/v2/novelinfo?id=%d", novel.ID), "")
	if deletion.Code != 204 {
		t.Fatalf("LT4 delete=%d %s", deletion.Code, deletion.Body)
	}
}
