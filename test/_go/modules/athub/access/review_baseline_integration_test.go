package athubaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	creatorstorage "github.com/AlbaTextus/atnexus/modules/creatplat/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Opt-in. The content module, the moderation adapter and the creator database
// are real and wired as the server wires them. A submission waits for review
// while the library moves on. Approving it writes what the submission itself
// changed, over whatever somebody else made of the same content in the
// meantime, and leaves the rest as the library holds it now. It is refused only
// when what it changes is no longer there to change.
func TestReviewBaselineIntegration(t *testing.T) {
	dsn := os.Getenv("OAUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set OAUTH_TEST_DATABASE_URL for the isolated review baseline integration")
	}
	accountURL := moderationSchema(t, dsn, "account", "../../../prisma/oauth/migrations/*/migration.sql")
	contentURL := moderationSchema(t, dsn, "content", "../../../prisma/athub/migrations/*/migration.sql")
	creatorURL := moderationSchema(t, dsn, "creator", "../../../prisma/creatplat/migrations/*/migration.sql")
	ctx := context.Background()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	accounts, err := store.NewPostgres(ctx, accountURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accounts.Close)
	creator, err := creatorstorage.New(ctx, creatorURL, accounts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(creator.Close)
	access, err := New(accounts, ":8003", quiet)
	if err != nil {
		t.Fatal(err)
	}
	access.EnableCreatorStorage(creator)
	hub, err := athub.New(ctx, athub.Config{DatabaseURL: contentURL, WrapHandler: access.Wrap, RecordChanges: access.RecordChanges}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hub.Close)
	reviews, err := pgx.Connect(ctx, creatorURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reviews.Close(context.Background()) })

	// LEVEL_2 submits for review and ADMIN decides. SUPER_ADMIN is everybody
	// else: its changes take effect at once.
	users, tokens := map[string]store.User{}, map[string]string{}
	for _, role := range []string{"LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		name := role
		user, err := accounts.UpsertUser(ctx, store.UserSeed{Email: strings.ToLower(role) + "@baseline.test", PasswordHash: "unused-test-hash", DisplayName: &name, Role: role})
		if err != nil {
			t.Fatal(err)
		}
		token := "ath_" + strings.Repeat(strings.ToLower(role)[:1], 42) + fmt.Sprint(len(tokens))
		if _, err := accounts.CreateATHubKey(ctx, store.ATHubKey{ID: uuid.New(), UserID: user.ID, Name: "integration", Prefix: "ath_test", TokenHash: security.HashToken(token)}); err != nil {
			t.Fatal(err)
		}
		users[role], tokens[role] = user, token
	}
	call := func(role, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var encoded []byte
		if body != nil {
			var err error
			if encoded, err = json.Marshal(body); err != nil {
				t.Fatal(err)
			}
		}
		request := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		request.Header.Set("Authorization", "Bearer "+tokens[role])
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		hub.Handler().ServeHTTP(response, request)
		return response
	}
	var platform struct {
		ID int32 `json:"id"`
	}
	if created := call("SUPER_ADMIN", "POST", "/api/v2/platforms", map[string]any{"name": "review-baseline"}); created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &platform) != nil {
		t.Fatalf("platform: %d %s", created.Code, created.Body)
	}
	submit := func(method, path string, body any) uuid.UUID {
		t.Helper()
		var queued struct {
			ID uuid.UUID `json:"review_id"`
		}
		if sent := call("LEVEL_2", method, path, body); sent.Code != 202 || json.Unmarshal(sent.Body.Bytes(), &queued) != nil {
			t.Fatalf("submission %s %s: %d %s", method, path, sent.Code, sent.Body)
		}
		return queued.ID
	}
	meanwhile := func(method, path string, body any) {
		t.Helper()
		if done := call("SUPER_ADMIN", method, path, body); done.Code < 200 || done.Code > 299 {
			t.Fatalf("%s %s: %d %s", method, path, done.Code, done.Body)
		}
	}
	approve := func(id uuid.UUID) store.ATHubReview {
		t.Helper()
		decided, err := creator.DecideATHubReview(ctx, users["ADMIN"].ID, id, "approve", "", access.ApplyReview)
		if err != nil {
			t.Fatal(err)
		}
		return decided
	}
	applied := func(decided store.ATHubReview) {
		t.Helper()
		if decided.Status != "approved" {
			t.Fatalf("approval=%s status=%v result=%s", decided.Status, decided.ResultStatus, decided.Result)
		}
	}
	refused := func(decided store.ATHubReview) {
		t.Helper()
		if decided.Status != "failed" || decided.ResultStatus == nil || *decided.ResultStatus != 412 || !strings.Contains(string(decided.Result), "submit it again") {
			t.Fatalf("approval=%s status=%v result=%s", decided.Status, decided.ResultStatus, decided.Result)
		}
	}
	// What the edit form sends: every field of the novel, changed or not.
	form := func(title, summary, coverURL string) map[string]any {
		return map[string]any{"platformId": platform.ID, "title": title, "summary": summary, "coverUrl": coverURL, "status": "ONGOING", "tagIds": []int32{}}
	}
	create := func() (int32, string) {
		t.Helper()
		var book struct {
			ID int32 `json:"id"`
		}
		if created := call("SUPER_ADMIN", "POST", "/api/v2/novelinfo", form("原标题", "原简介", "https://covers.example/a.jpg")); created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &book) != nil {
			t.Fatalf("novel: %d %s", created.Code, created.Body)
		}
		return book.ID, fmt.Sprintf("/api/v2/novelinfo?id=%d", book.ID)
	}
	shown := func(path string) [3]string {
		t.Helper()
		var book struct {
			Title    string `json:"title"`
			Summary  string `json:"summary"`
			CoverURL string `json:"coverUrl"`
		}
		if read := call("SUPER_ADMIN", "GET", path, nil); read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &book) != nil {
			t.Fatalf("read %s: %d %s", path, read.Code, read.Body)
		}
		return [3]string{book.Title, book.Summary, book.CoverURL}
	}
	chapter := func(novel, order int32, title, content string) map[string]any {
		return map[string]any{"novelId": novel, "order": order, "title": title, "content": content}
	}
	read := func(novel, order int32) [2]string {
		t.Helper()
		var stored struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		path := fmt.Sprintf("/api/v2/content?novelId=%d&order=%d", novel, order)
		if found := call("SUPER_ADMIN", "GET", path, nil); found.Code != 200 || json.Unmarshal(found.Body.Bytes(), &stored) != nil {
			t.Fatalf("read %s: %d %s", path, found.Code, found.Body)
		}
		return [2]string{stored.Title, stored.Content}
	}
	// The changes the history of a novel credits the submitter with.
	credited := func(novel int32) []athub.Change {
		t.Helper()
		history, err := creator.NovelHistory(ctx, novel, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		var changes []athub.Change
		for _, revision := range history.Revisions {
			if !strings.Contains(string(revision.Contributor), users["LEVEL_2"].ID.String()) {
				continue
			}
			var recorded []athub.Change
			if err := json.Unmarshal(revision.Changes, &recorded); err != nil {
				t.Fatal(err)
			}
			changes = append(changes, recorded...)
		}
		return changes
	}

	t.Run("fields the submission left alone keep what the library holds now", func(t *testing.T) {
		id, book := create()
		pending := submit("PUT", book, form("新标题", "原简介", "https://covers.example/a.jpg"))
		meanwhile("PUT", book, form("原标题", "别人改的简介", "https://covers.example/b.jpg"))
		applied(approve(pending))
		if got := shown(book); got != [3]string{"新标题", "别人改的简介", "https://covers.example/b.jpg"} {
			t.Fatalf("novel=%q", got)
		}
		// The history credits the submitter with the title and nothing else.
		changes := credited(id)
		if len(changes) != 1 || changes[0].Entity != "Novel" || changes[0].Action != "UPDATE" {
			t.Fatalf("changes=%+v", changes)
		}
		before, after := changes[0].Before, changes[0].After
		if before["title"] != "原标题" || after["title"] != "新标题" ||
			before["summary"] != "别人改的简介" || after["summary"] != "别人改的简介" ||
			before["coverUrl"] != "https://covers.example/b.jpg" || after["coverUrl"] != "https://covers.example/b.jpg" {
			t.Fatalf("before=%v after=%v", before, after)
		}
	})

	t.Run("a field somebody else changed meanwhile is written over", func(t *testing.T) {
		id, book := create()
		pending := submit("PUT", book, form("新标题", "新简介", "https://covers.example/a.jpg"))
		meanwhile("PUT", book, form("别人改的标题", "原简介", "https://covers.example/b.jpg"))
		applied(approve(pending))
		// The title and the summary are the submission's. The cover, which the
		// submission left alone, is the one somebody else chose.
		if got := shown(book); got != [3]string{"新标题", "新简介", "https://covers.example/b.jpg"} {
			t.Fatalf("novel=%q", got)
		}
		// The history shows the title that was written over.
		changes := credited(id)
		if len(changes) != 1 || changes[0].Entity != "Novel" || changes[0].Action != "UPDATE" {
			t.Fatalf("changes=%+v", changes)
		}
		before, after := changes[0].Before, changes[0].After
		if before["title"] != "别人改的标题" || after["title"] != "新标题" ||
			before["summary"] != "原简介" || after["summary"] != "新简介" ||
			before["coverUrl"] != "https://covers.example/b.jpg" || after["coverUrl"] != "https://covers.example/b.jpg" {
			t.Fatalf("before=%v after=%v", before, after)
		}
	})

	t.Run("of two pending edits to one field the one approved later stands", func(t *testing.T) {
		for _, order := range [][2]int{{0, 1}, {1, 0}} {
			_, book := create()
			titles := [2]string{"第一份修改", "第二份修改"}
			var pending [2]uuid.UUID
			for i, title := range titles {
				pending[i] = submit("PUT", book, form(title, "原简介", "https://covers.example/a.jpg"))
			}
			for _, turn := range order {
				applied(approve(pending[turn]))
				if got := shown(book); got[0] != titles[turn] {
					t.Fatalf("approved %q, novel=%q", titles[turn], got)
				}
			}
		}
	})

	t.Run("chapter edits", func(t *testing.T) {
		id, _ := create()
		path := fmt.Sprintf("/api/v2/content?novelId=%d&order=1", id)
		meanwhile("PUT", "/api/v2/content", chapter(id, 1, "第一章", "原来的正文。"))
		// Only the title: the body somebody else rewrote meanwhile stays.
		pending := submit("PUT", "/api/v2/content", chapter(id, 1, "第一章 修订", "原来的正文。"))
		meanwhile("PUT", "/api/v2/content", chapter(id, 1, "第一章", "别人重写的正文。"))
		applied(approve(pending))
		if got := read(id, 1); got != [2]string{"第一章 修订", "别人重写的正文。"} {
			t.Fatalf("chapter=%q", got)
		}
		// Only the body: it is written over the one somebody else wrote
		// meanwhile, and the title somebody else chose stays.
		pending = submit("PUT", "/api/v2/content", chapter(id, 1, "第一章 修订", "投稿者重写的正文。"))
		meanwhile("PUT", "/api/v2/content", chapter(id, 1, "别人改的章节名", "别人又改了一次正文，这次写得长了不少。"))
		applied(approve(pending))
		if got := read(id, 1); got != [2]string{"别人改的章节名", "投稿者重写的正文。"} {
			t.Fatalf("chapter=%q", got)
		}
		// A body rewritten into one of the same length is applied over a body
		// of another length all the same.
		pending = submit("PUT", "/api/v2/content", chapter(id, 1, "别人改的章节名", "投稿者改写的正文。"))
		meanwhile("PUT", "/api/v2/content", chapter(id, 1, "别人改的章节名", "短正文。"))
		applied(approve(pending))
		if got := read(id, 1); got != [2]string{"别人改的章节名", "投稿者改写的正文。"} {
			t.Fatalf("chapter=%q", got)
		}
		// A chapter that was removed meanwhile is not brought back. Neither is one
		// that was removed and written again changed: it is another chapter.
		removed := submit("PUT", "/api/v2/content", chapter(id, 1, "投稿者改的章节名", "投稿者改写的正文。"))
		replaced := submit("PUT", "/api/v2/content", chapter(id, 1, "投稿者再改的章节名", "投稿者改写的正文。"))
		meanwhile("DELETE", path, nil)
		refused(approve(removed))
		if gone := call("SUPER_ADMIN", "GET", path, nil); gone.Code != 404 {
			t.Fatalf("removed chapter: %d %s", gone.Code, gone.Body)
		}
		meanwhile("PUT", "/api/v2/content", chapter(id, 1, "别人改的章节名", "投稿者改写的正文。"))
		refused(approve(replaced))
		if got := read(id, 1); got != [2]string{"别人改的章节名", "投稿者改写的正文。"} {
			t.Fatalf("chapter=%q", got)
		}
	})

	t.Run("uploads into an existing novel", func(t *testing.T) {
		id, _ := create()
		meanwhile("PUT", "/api/v2/content", chapter(id, 1, "第一章", "原来的正文。"))
		upload := func(chapters ...map[string]any) map[string]any {
			return map[string]any{"novelId": id, "volumes": []any{}, "chapters": chapters}
		}
		piece := func(order int32, title, content string) map[string]any {
			return map[string]any{"order": order, "title": title, "content": content}
		}
		// The upload repeats a chapter as it was and adds one. The repeated
		// chapter was rewritten meanwhile: it is left as it is now.
		pending := submit("POST", "/api/v2/upload", upload(piece(1, "第一章", "原来的正文。"), piece(2, "第二章", "投稿的第二章。")))
		meanwhile("PUT", "/api/v2/content", chapter(id, 1, "第一章", "别人重写的正文。"))
		applied(approve(pending))
		if first, second := read(id, 1), read(id, 2); first != [2]string{"第一章", "别人重写的正文。"} || second != [2]string{"第二章", "投稿的第二章。"} {
			t.Fatalf("chapters=%q %q", first, second)
		}
		// A chapter somebody else added at the same position is not overwritten.
		pending = submit("POST", "/api/v2/upload", upload(piece(3, "第三章", "投稿的第三章。")))
		meanwhile("PUT", "/api/v2/content", chapter(id, 3, "第三章", "别人先写的第三章。"))
		refused(approve(pending))
		if got := read(id, 3); got != [2]string{"第三章", "别人先写的第三章。"} {
			t.Fatalf("chapter=%q", got)
		}
	})

	t.Run("a submission that creates a novel is applied whole", func(t *testing.T) {
		pending := submit("POST", "/api/v2/upload", map[string]any{
			"novel":    form("投稿的新作品", "投稿的简介", "https://covers.example/new.jpg"),
			"volumes":  []any{},
			"chapters": []any{map[string]any{"order": 1, "title": "第一章", "content": "新作品的正文。"}},
		})
		decided := approve(pending)
		applied(decided)
		var uploaded struct {
			Novel struct {
				ID int32 `json:"id"`
			} `json:"novel"`
		}
		if err := json.Unmarshal(decided.Result, &uploaded); err != nil || uploaded.Novel.ID <= 0 {
			t.Fatalf("result=%s err=%v", decided.Result, err)
		}
		if got := read(uploaded.Novel.ID, 1); got != [2]string{"第一章", "新作品的正文。"} {
			t.Fatalf("chapter=%q", got)
		}
	})

	// Reviews queued before submissions recorded what they change carry no
	// such record. They are applied in full, as they always were.
	t.Run("a review without a record of what it changes overwrites", func(t *testing.T) {
		_, book := create()
		pending := submit("PUT", book, form("新标题", "原简介", "https://covers.example/a.jpg"))
		forgotten, err := reviews.Exec(ctx, `UPDATE athub_reviews SET baseline = NULL WHERE id = $1 AND baseline IS NOT NULL`, pending)
		if err != nil || forgotten.RowsAffected() != 1 {
			t.Fatalf("the submission did not record what it changes: rows=%d err=%v", forgotten.RowsAffected(), err)
		}
		meanwhile("PUT", book, form("原标题", "别人改的简介", "https://covers.example/b.jpg"))
		applied(approve(pending))
		if got := shown(book); got != [3]string{"新标题", "原简介", "https://covers.example/a.jpg"} {
			t.Fatalf("novel=%q", got)
		}
	})
}
