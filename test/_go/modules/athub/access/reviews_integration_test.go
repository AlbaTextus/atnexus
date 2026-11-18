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

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
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
	accountURL := moderationSchema(t, dsn, "account", "../../../prisma/oauth/migrations/*/migration.sql")
	contentURL := moderationSchema(t, dsn, "content", "../../../prisma/athub/migrations/*/migration.sql")
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

	t.Run("browser session novel edits", func(t *testing.T) {
		created := call("SUPER_ADMIN", "POST", "/api/v2/novelinfo", body)
		var book struct {
			ID int32 `json:"id"`
		}
		if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &book) != nil || book.ID <= 0 {
			t.Fatalf("edit fixture: %d %s", created.Code, created.Body)
		}
		cookie := "browser-edit-" + uuid.NewString()
		if err := p.CreateSession(ctx, users["LEVEL_2"].ID, security.HashToken(cookie), time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		session, err := p.SessionByHash(ctx, security.HashToken(cookie))
		if err != nil {
			t.Fatal(err)
		}
		submitEdit := func(title string) store.ATHubReview {
			t.Helper()
			payload := fmt.Sprintf(`{"platformId":%d,"title":%q,"status":"COMPLETED","summary":"浏览器编辑简介","tagIds":[]}`, platformData.ID, title)
			request := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/account/athub/novels/%d", book.ID), strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			access.SubmitNovelEdit(response, request, users["LEVEL_2"].ID, session.ID, book.ID)
			var queued struct {
				ID uuid.UUID `json:"review_id"`
			}
			if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &queued) != nil {
				t.Fatalf("browser edit: %d %s", response.Code, response.Body)
			}
			item, err := p.ATHubReview(ctx, users["LEVEL_2"].ID, queued.ID, false)
			if err != nil || item.Operation != "edit" || item.Status != "pending" || item.Query != fmt.Sprintf("id=%d", book.ID) {
				t.Fatalf("browser edit review: %+v %v", item, err)
			}
			return item
		}
		readTitle := func(want string) {
			t.Helper()
			visible := call("USER", "GET", fmt.Sprintf("/api/v2/novelinfo?id=%d", book.ID), "")
			var data struct {
				Title string `json:"title"`
			}
			if visible.Code != 200 || json.Unmarshal(visible.Body.Bytes(), &data) != nil || data.Title != want {
				t.Fatalf("visible title should be %q: %d %s", want, visible.Code, visible.Body)
			}
		}
		item := submitEdit("审核通过的浏览器编辑")
		readTitle("等待审核的小说")
		if _, err := p.DecideATHubReview(ctx, users["LEVEL_2"].ID, item.ID, "approve", "", access.ApplyReview); err == nil {
			t.Fatal("submitter approved own edit")
		}
		approved, err := p.DecideATHubReview(ctx, users["ADMIN"].ID, item.ID, "approve", "", access.ApplyReview)
		if err != nil || approved.Status != "approved" {
			t.Fatalf("browser edit approval: %+v %v", approved, err)
		}
		readTitle("审核通过的浏览器编辑")
		history, err := p.NovelHistory(ctx, book.ID, 20, 0)
		if err != nil || history.Total != 2 || !strings.Contains(string(history.Revisions[0].Contributor), users["LEVEL_2"].ID.String()) {
			t.Fatalf("browser edit contributor lost: %+v %v", history, err)
		}
		item = submitEdit("驳回的浏览器编辑")
		rejected, err := p.DecideATHubReview(ctx, users["ADMIN"].ID, item.ID, "reject", "资料不准确", access.ApplyReview)
		if err != nil || rejected.Status != "rejected" {
			t.Fatalf("browser edit rejection: %+v %v", rejected, err)
		}
		readTitle("审核通过的浏览器编辑")
	})

	t.Run("browser session submissions", func(t *testing.T) {
		for _, role := range []string{"USER", "LEVEL_2", "ADMIN"} {
			keyCallsBefore, err := p.ListATHubCalls(ctx, users[role].ID, 1, 0)
			if err != nil || len(keyCallsBefore) != 1 {
				t.Fatalf("missing API key call fixture: %+v %v", keyCallsBefore, err)
			}
			cookie := "browser-submission-" + uuid.NewString()
			if err := p.CreateSession(ctx, users[role].ID, security.HashToken(cookie), time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			session, err := p.SessionByHash(ctx, security.HashToken(cookie))
			if err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"novel":{"platformId":%d,"title":"%s浏览器投稿","status":"ONGOING"},"chapters":[{"order":1,"title":"第一章","content":"这是通过登录会话提交的正文。"}],"volumes":[]}`, platformData.ID, role)
			request := httptest.NewRequest("POST", "/api/v1/account/athub/submissions", strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-AlbaTextus-Client", "account-center")
			response := httptest.NewRecorder()
			access.SubmitUpload(response, request, users[role].ID, session.ID)
			want := map[string]int{"USER": 403, "LEVEL_2": 202, "ADMIN": 201}[role]
			if response.Code != want {
				t.Fatalf("role=%s status=%d body=%s", role, response.Code, response.Body)
			}
			if role == "USER" {
				continue
			}
			body := response.Body.Bytes()
			if role == "LEVEL_2" {
				var queued struct {
					ID uuid.UUID `json:"review_id"`
				}
				if err := json.Unmarshal(body, &queued); err != nil {
					t.Fatal(err)
				}
				item, err := p.ATHubReview(ctx, users[role].ID, queued.ID, false)
				if err != nil || item.Status != "pending" || item.Path != "/api/v2/upload" {
					t.Fatalf("browser submission missing from own reviews: %+v %v", item, err)
				}
				approved, err := p.DecideATHubReview(ctx, users["ADMIN"].ID, queued.ID, "approve", "", access.ApplyReview)
				if err != nil || approved.Status != "approved" {
					t.Fatalf("browser approval: %+v %v", approved, err)
				}
				body = approved.Result
			}
			var uploaded struct {
				Novel struct {
					ID int32 `json:"id"`
				} `json:"novel"`
				ChapterCount int `json:"chapterCount"`
			}
			if err := json.Unmarshal(body, &uploaded); err != nil || uploaded.Novel.ID <= 0 || uploaded.ChapterCount != 1 {
				t.Fatalf("invalid upload result: %s err=%v", body, err)
			}
			history, err := p.NovelHistory(ctx, uploaded.Novel.ID, 20, 0)
			if err != nil || history.Total != 1 || !strings.Contains(string(history.Revisions[0].Contributor), users[role].ID.String()) {
				t.Fatalf("browser contributor lost: %+v %v", history, err)
			}
			calls, err := p.ListATHubCalls(ctx, users[role].ID, 1, 0)
			if err != nil || len(calls) != 1 || calls[0].KeyID == nil || calls[0].ID != keyCallsBefore[0].ID {
				t.Fatalf("browser submission counted against personal key: %+v %v", calls, err)
			}
		}
	})
}
