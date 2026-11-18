package creatplat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

type recordedHistory struct {
	store.NovelHistoryStore
	history      store.NovelHistory
	contributors json.RawMessage
}

func (h *recordedHistory) NovelHistory(context.Context, int32, int, int) (store.NovelHistory, error) {
	// Every read is a fresh result, as from the database.
	fresh := h.history
	fresh.Revisions = append([]store.NovelRevision(nil), h.history.Revisions...)
	return fresh, nil
}
func (h *recordedHistory) NovelContributors(context.Context, int32) (json.RawMessage, error) {
	return h.contributors, nil
}

// A revision records the avatar its contributor had then, and that file may be
// gone. Replies carry the account's present avatar instead.
func TestHistoryShowsCurrentAvatars(t *testing.T) {
	known, unknown := uuid.New(), uuid.New()
	person := func(id, avatar string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"id": id, "name": "贡献者", "avatar": avatar, "edits": 3})
		return raw
	}
	reads := &recordedHistory{
		history: store.NovelHistory{Total: 3, Revisions: []store.NovelRevision{
			{ID: uuid.New(), Contributor: person(known.String(), "https://images.test/old.png"), Changes: json.RawMessage(`[]`), ChangeStats: json.RawMessage(`[]`)},
			{ID: uuid.New(), Contributor: person(unknown.String(), "https://images.test/left.png"), Changes: json.RawMessage(`[]`), ChangeStats: json.RawMessage(`[]`)},
			{ID: uuid.New(), Contributor: json.RawMessage(`{"name":"未记录账号"}`), Changes: json.RawMessage(`[]`), ChangeStats: json.RawMessage(`[]`)},
		}},
		contributors: json.RawMessage(`{"creator":` + string(person(known.String(), "https://images.test/old.png")) + `,"contributors":[` + string(person(unknown.String(), "https://images.test/left.png")) + `]}`),
	}
	get := func(path string, lookup func(context.Context, []uuid.UUID) (map[uuid.UUID]string, error)) string {
		t.Helper()
		history := NewHistory(reads, nil)
		if lookup != nil {
			history.ShowCurrentAvatars(lookup)
		}
		w := httptest.NewRecorder()
		history.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"?novelId=2", nil))
		if w.Code != 200 {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body)
		}
		return w.Body.String()
	}
	asked := 0
	current := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
		asked++
		if len(ids) != 2 {
			t.Fatalf("looked up %v", ids)
		}
		return map[uuid.UUID]string{known: "https://images.test/new.png"}, nil
	}
	for _, path := range []string{"/api/v3/history", "/api/v3/contributors"} {
		body := get(path, current)
		// A deleted account keeps what was recorded; extra fields survive.
		if strings.Contains(body, "old.png") || !strings.Contains(body, "new.png") || !strings.Contains(body, "left.png") || !strings.Contains(body, `"edits":3`) {
			t.Fatalf("%s body=%s", path, body)
		}
		if recorded := get(path, nil); !strings.Contains(recorded, "old.png") {
			t.Fatalf("%s without lookup: %s", path, recorded)
		}
		failing := func(context.Context, []uuid.UUID) (map[uuid.UUID]string, error) { return nil, errors.New("down") }
		if recorded := get(path, failing); !strings.Contains(recorded, "old.png") {
			t.Fatalf("%s with failed lookup: %s", path, recorded)
		}
	}
	if asked != 2 {
		t.Fatalf("lookups=%d", asked)
	}
	reads.contributors = json.RawMessage(`{"creator":null,"contributors":[]}`)
	if body := strings.TrimSpace(get("/api/v3/contributors", current)); body != `{"creator":null,"contributors":[]}` {
		t.Fatalf("empty board became %s", body)
	}
}
