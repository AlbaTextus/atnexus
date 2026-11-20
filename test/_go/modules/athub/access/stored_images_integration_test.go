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
)

// imageShelf stands in for the image host: it only remembers which files it
// stores. Another spelling of a stored address carries a query.
type imageShelf map[string]bool

func (s imageShelf) Address(location string) string {
	if !strings.HasPrefix(location, hostedFiles) {
		return ""
	}
	address, _, _ := strings.Cut(location, "?")
	return address
}

func (s imageShelf) Missing(_ context.Context, location string) (bool, error) {
	address := s.Address(location)
	return address != "" && !s[address], nil
}

func (s imageShelf) Remove(_ context.Context, _, _, location string) error {
	delete(s, location)
	return nil
}

// Opt-in. The content module, the moderation adapter, and the creator database
// are real and wired as the server wires them; only the image host is a
// stand-in. A replaced cover is deleted once nothing refers to it, and nothing
// can start to point at a deleted image afterwards.
func TestStoredImagesIntegration(t *testing.T) {
	dsn := os.Getenv("OAUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set OAUTH_TEST_DATABASE_URL for the isolated stored image integration")
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
	access := New(accounts, quiet)
	access.EnableCreatorStorage(creator)
	hub, err := athub.New(ctx, athub.Config{DatabaseURL: contentURL, WrapHandler: access.Wrap, RecordChanges: access.RecordChanges}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hub.Close)
	cover := func(name string) string { return hostedFiles + "book/u/covers/" + name + ".jpg" }
	shelf := imageShelf{}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		shelf[cover(name)] = true
	}
	access.EnableImageCleanup(shelf, hub)

	users, tokens := map[string]store.User{}, map[string]string{}
	for _, role := range []string{"LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		name := role
		user, err := accounts.UpsertUser(ctx, store.UserSeed{Email: strings.ToLower(role) + "@images.test", PasswordHash: "unused-test-hash", DisplayName: &name, Role: role})
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
	if created := call("SUPER_ADMIN", "POST", "/api/v2/platforms", map[string]any{"name": "stored-images"}); created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &platform) != nil {
		t.Fatalf("platform: %d %s", created.Code, created.Body)
	}
	novel := func(title, coverURL string) map[string]any {
		return map[string]any{"platformId": platform.ID, "title": title, "status": "ONGOING", "coverUrl": coverURL}
	}
	create := func(title, coverURL string) string {
		t.Helper()
		var book struct {
			ID int32 `json:"id"`
		}
		if created := call("SUPER_ADMIN", "POST", "/api/v2/novelinfo", novel(title, coverURL)); created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &book) != nil {
			t.Fatalf("novel %s: %d %s", title, created.Code, created.Body)
		}
		return fmt.Sprintf("/api/v2/novelinfo?id=%d", book.ID)
	}
	shown := func(path string) string {
		t.Helper()
		var book struct {
			CoverURL string `json:"coverUrl"`
		}
		if read := call("SUPER_ADMIN", "GET", path, nil); read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &book) != nil {
			t.Fatalf("read %s: %d %s", path, read.Code, read.Body)
		}
		return book.CoverURL
	}
	replace := func(path, coverURL string) int {
		t.Helper()
		return call("SUPER_ADMIN", "PUT", path, novel("封面", coverURL)).Code
	}
	submit := func(path, coverURL string) uuid.UUID {
		t.Helper()
		var queued struct {
			ID uuid.UUID `json:"review_id"`
		}
		if sent := call("LEVEL_2", "PUT", path, novel("待审的修改", coverURL)); sent.Code != 202 || json.Unmarshal(sent.Body.Bytes(), &queued) != nil {
			t.Fatalf("submission: %d %s", sent.Code, sent.Body)
		}
		return queued.ID
	}
	approve := func(id uuid.UUID) store.ATHubReview {
		t.Helper()
		decided, err := creator.DecideATHubReview(ctx, users["ADMIN"].ID, id, "approve", "", access.ApplyReview)
		if err != nil {
			t.Fatal(err)
		}
		return decided
	}

	// A new novel may only show a cover the host stores.
	if refused := call("SUPER_ADMIN", "POST", "/api/v2/novelinfo", novel("没有封面文件", cover("never"))); refused.Code != 422 {
		t.Fatalf("a novel was created with a cover that is not stored: %d %s", refused.Code, refused.Body)
	}
	book := create("第一本", cover("a"))

	// The replaced cover is deleted; the form that still carries it cannot bring it back.
	if status := replace(book, cover("b")); status != 200 || shelf[cover("a")] || !shelf[cover("b")] {
		t.Fatalf("status=%d stored=%v", status, shelf)
	}
	stale := call("SUPER_ADMIN", "PUT", book, novel("旧表单", cover("a")))
	if stale.Code != 422 || !strings.Contains(stale.Body.String(), "upload it again") {
		t.Fatalf("stale form: %d %s", stale.Code, stale.Body)
	}
	if shown(book) != cover("b") || !shelf[cover("b")] {
		t.Fatalf("a refused change was saved: cover=%q stored=%v", shown(book), shelf)
	}
	// A cover left unchanged is not this change's doing, even if its file is gone.
	delete(shelf, cover("b"))
	if edited := call("SUPER_ADMIN", "PUT", book, novel("只改标题", cover("b"))); edited.Code != 200 {
		t.Fatalf("an unrelated edit was refused: %d %s", edited.Code, edited.Body)
	}
	shelf[cover("b")] = true

	// A cover another novel still shows, in whatever spelling, is kept.
	other := create("第二本", cover("b")+"?v=2")
	if status := replace(book, cover("c")); status != 200 || !shelf[cover("b")] {
		t.Fatalf("status=%d stored=%v", status, shelf)
	}
	if shown(other) != cover("b")+"?v=2" {
		t.Fatalf("the other novel shows %q", shown(other))
	}

	// So is a cover that a pending review carries.
	pending := submit(book, cover("c"))
	if status := replace(book, cover("d")); status != 200 || !shelf[cover("c")] {
		t.Fatalf("status=%d stored=%v", status, shelf)
	}
	// That review changed the title and only repeated the cover it saw, so
	// approving it does not write the older cover back over the newer one.
	if decided := approve(pending); decided.Status != "approved" || shown(book) != cover("d") {
		t.Fatalf("approval=%s cover=%q", decided.Status, shown(book))
	}
	// No cover was replaced, so none is deleted: the one the review carried
	// stays on the host although nothing shows it any more.
	if !shelf[cover("d")] || !shelf[cover("c")] {
		t.Fatalf("stored=%v", shelf)
	}

	// A review whose image was deleted while it waited is refused when approved.
	pending = submit(book, cover("e"))
	delete(shelf, cover("e"))
	decided := approve(pending)
	if decided.Status != "failed" || decided.ResultStatus == nil || *decided.ResultStatus != 422 {
		t.Fatalf("approval=%+v", decided)
	}
	if shown(book) != cover("d") || !shelf[cover("d")] {
		t.Fatalf("a refused review was applied: cover=%q stored=%v", shown(book), shelf)
	}

	// Deleting a novel deletes its cover.
	if removed := call("SUPER_ADMIN", "DELETE", book, nil); removed.Code != 204 || shelf[cover("d")] {
		t.Fatalf("status=%d stored=%v", removed.Code, shelf)
	}
	// Only the address a file was stored under deletes it: a cover recorded in
	// another spelling is left on the host.
	if removed := call("SUPER_ADMIN", "DELETE", other, nil); removed.Code != 204 || !shelf[cover("b")] {
		t.Fatalf("status=%d stored=%v", removed.Code, shelf)
	}
}
