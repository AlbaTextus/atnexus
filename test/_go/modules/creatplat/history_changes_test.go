package creatplat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/creatplat/repository"
	"github.com/google/uuid"
)

type historyReads struct {
	store.NovelHistoryStore
	known                uuid.UUID
	novel                int32
	limit, offset, lists int
}

func (h *historyReads) NovelHistory(context.Context, int32, int, int) (store.NovelHistory, error) {
	h.lists++
	return store.NovelHistory{}, nil
}

func (h *historyReads) NovelRevisionChanges(_ context.Context, novel int32, revision uuid.UUID, limit, offset int) (repository.RevisionChanges, error) {
	if revision != h.known {
		return repository.RevisionChanges{}, store.ErrNotFound
	}
	h.novel, h.limit, h.offset = novel, limit, offset
	return repository.RevisionChanges{Changes: json.RawMessage(`[{"entity":"Chapter"}]`), Total: 250}, nil
}

func TestHistoryServesOneEntryChangesByPage(t *testing.T) {
	reads := &historyReads{known: uuid.New()}
	get := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		NewHistory(reads, nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v3/history?"+query, nil))
		return w
	}
	w := get("novelId=7&revision=" + reads.known.String() + "&limit=100&offset=200")
	var page struct {
		Changes []map[string]string `json:"changes"`
		Total   int64               `json:"total"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Total != 250 || len(page.Changes) != 1 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	if reads.novel != 7 || reads.limit != 100 || reads.offset != 200 || reads.lists != 0 {
		t.Fatalf("reads=%+v", reads)
	}
	for query, want := range map[string]int{
		"novelId=7&revision=nope":                            400,
		"novelId=7&revision=" + uuid.NewString():             404,
		"novelId=7&limit=101&revision=" + uuid.NewString():   400,
		"novelId=7&revision=" + reads.known.String() + "&x=": 400,
	} {
		if got := get(query); got.Code != want {
			t.Errorf("%s: status=%d want %d", query, got.Code, want)
		}
	}
	// Contributors take no paging or revision parameter.
	w = httptest.NewRecorder()
	NewHistory(reads, nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v3/contributors?novelId=7&revision="+reads.known.String(), nil))
	if w.Code != 400 {
		t.Fatalf("contributors status=%d", w.Code)
	}
}
