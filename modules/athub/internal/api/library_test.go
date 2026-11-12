package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"albatextus/athub/internal/storage"
)

type libraryStore struct {
	fakeStore
	page storage.NovelPageFilter
	ids  []int32
}

func (s *libraryStore) ListNovelPage(_ context.Context, f storage.NovelPageFilter) (storage.NovelPage, error) {
	s.page = f
	return storage.NovelPage{Items: []storage.Novel{{ID: 9}}, Total: 125}, nil
}
func (s *libraryStore) AuthorsByIDs(_ context.Context, ids []int32) ([]storage.Author, error) {
	s.ids = ids
	return []storage.Author{{ID: ids[0]}}, nil
}
func (s *libraryStore) CatalogCount(context.Context, string) (int64, error) { return 125, nil }

func TestCatalogPageAndBatchAuthors(t *testing.T) {
	st := &libraryStore{}
	handler := testHandler(t, st)
	w := request(handler, "GET", "/api/v2/novelinfo?view=page&limit=24&offset=24&q=writer&sort=title&authorId=3&tagId=2", "", "Bearer "+testToken)
	var page storage.NovelPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != 200 || page.Total != 125 || len(page.Items) != 1 {
		t.Fatalf("page status=%d body=%s error=%v", w.Code, w.Body, err)
	}
	if st.page.Limit != 24 || st.page.Offset != 24 || st.page.AuthorID != 3 || st.page.TagID != 2 || st.page.Search != "writer" || st.page.Sort != "title" {
		t.Fatalf("invalid filter: %+v", st.page)
	}
	w = request(handler, "GET", "/api/v2/authors?ids=3,5,8", "", "Bearer "+testToken)
	if w.Code != 200 || !reflect.DeepEqual(st.ids, []int32{3, 5, 8}) {
		t.Fatalf("batch authors status=%d ids=%v", w.Code, st.ids)
	}
	for _, path := range []string{"novelinfo", "authors", "tags", "platforms"} {
		w = request(handler, "GET", "/api/v2/"+path+"?view=count", "", "Bearer "+testToken)
		if w.Code != 200 || w.Body.String() != "{\"total\":125}\n" {
			t.Fatalf("count %s: %d %s", path, w.Code, w.Body)
		}
	}
	for _, path := range []string{
		"/api/v2/novelinfo?view=page&limit=101", "/api/v2/novelinfo?view=page&sort=bad",
		"/api/v2/novelinfo?view=page&authorId=-1", "/api/v2/novelinfo?view=page&id=1",
		"/api/v2/novelinfo?view=page&q=%00", "/api/v2/novelinfo?view=page&view=count",
		"/api/v2/novelinfo?view=count&limit=1", "/api/v2/tags?view=other",
		"/api/v2/authors?ids=0", "/api/v2/authors?ids=1&platformId=3",
	} {
		if w := request(handler, http.MethodGet, path, "", "Bearer "+testToken); w.Code != 400 {
			t.Errorf("invalid query %s: %d %s", path, w.Code, w.Body)
		}
	}
	// Legacy callers still receive an array and use the original store method.
	w = request(handler, "GET", "/api/v2/novelinfo?limit=24", "", "Bearer "+testToken)
	if w.Code != 200 || w.Body.String() != "[]\n" || st.called != "listNovels" {
		t.Fatalf("legacy API changed: %d %s", w.Code, w.Body)
	}
}
