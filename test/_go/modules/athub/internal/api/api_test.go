package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

var testToken = strings.Repeat("t", 32)

type fakeStore struct {
	called                   string
	err                      error
	novel                    storage.NovelInput
	chapter                  storage.ChapterInput
	filter                   storage.NovelFilter
	id, order, limit, offset int32
}

func (s *fakeStore) GetNovel(_ context.Context, id int32) (storage.Novel, error) {
	s.called, s.id = "getNovel", id
	return storage.Novel{ID: id}, s.err
}
func (s *fakeStore) ListNovels(_ context.Context, f storage.NovelFilter) ([]storage.Novel, error) {
	s.called, s.filter = "listNovels", f
	return nil, s.err
}
func (s *fakeStore) CreateNovel(_ context.Context, n storage.NovelInput) (storage.Novel, error) {
	s.called, s.novel = "createNovel", n
	return storage.Novel{ID: 7, NovelInput: n}, s.err
}
func (s *fakeStore) UpdateNovel(_ context.Context, id int32, n storage.NovelInput) (storage.Novel, error) {
	s.called, s.id, s.novel = "updateNovel", id, n
	return storage.Novel{ID: id, NovelInput: n}, s.err
}
func (s *fakeStore) DeleteNovel(_ context.Context, id int32) error {
	s.called, s.id = "deleteNovel", id
	return s.err
}
func (s *fakeStore) GetChapter(_ context.Context, id, order int32) (storage.Chapter, error) {
	s.called, s.id, s.order = "getChapter", id, order
	return storage.Chapter{ID: 8, ChapterInput: storage.ChapterInput{NovelID: id, Order: order}}, s.err
}
func (s *fakeStore) ListChapters(_ context.Context, id, limit, offset int32) ([]storage.Chapter, error) {
	s.called, s.id, s.limit, s.offset = "listChapters", id, limit, offset
	return nil, s.err
}
func (s *fakeStore) SaveChapter(_ context.Context, c storage.ChapterInput) (storage.Chapter, error) {
	s.called, s.chapter = "saveChapter", c
	return storage.Chapter{ID: 8, ChapterInput: c}, s.err
}
func (s *fakeStore) DeleteChapter(_ context.Context, id, order int32) error {
	s.called, s.id, s.order = "deleteChapter", id, order
	return s.err
}

func testHandler(t *testing.T, store storage.Store) http.Handler {
	t.Helper()
	handler, err := New(store, testToken, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func request(handler http.Handler, method, path, body, auth string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestTokenConfiguration(t *testing.T) {
	for _, token := range []string{"", "short", " " + testToken, testToken + "\n", testToken + " space"} {
		if _, err := New(nil, token, nil); err == nil {
			t.Errorf("accepted invalid token %q", token)
		}
	}
}

func TestAuthentication(t *testing.T) {
	handler := testHandler(t, nil)
	for _, auth := range []string{"", "Basic " + testToken, "Bearer wrong", "Bearer " + testToken + " ", "Bearer  " + testToken} {
		w := request(handler, http.MethodGet, "/api/v2/novelinfo", "", auth)
		if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("auth %q: status %d", auth, w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v2/novelinfo", nil)
	r.Header.Add("Authorization", "Bearer "+testToken)
	r.Header.Add("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("duplicate authorization: status %d", w.Code)
	}
}

func TestRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, called string
		status                     int
	}{
		{"GET", "/api/v2/novelinfo?id=7", "", "getNovel", 200},
		{"GET", "/api/v2/novelinfo?platformId=2&authorId=3&limit=5&offset=10", "", "listNovels", 200},
		{"POST", "/api/v2/novelinfo", `{"platformId":2,"title":" Book "}`, "createNovel", 201},
		{"PUT", "/api/v2/novelinfo?id=7", `{"platformId":2,"title":"Book","status":"COMPLETED","authorId":3}`, "updateNovel", 200},
		{"DELETE", "/api/v2/novelinfo?id=7", "", "deleteNovel", 204},
		{"GET", "/api/v2/content?novelId=7&order=2", "", "getChapter", 200},
		{"GET", "/api/v2/content?novelId=7&limit=3&offset=4", "", "listChapters", 200},
		{"PUT", "/api/v2/content", `{"novelId":7,"order":2,"title":" Chapter ","content":"  body\n"}`, "saveChapter", 200},
		{"DELETE", "/api/v2/content?novelId=7&order=2", "", "deleteChapter", 204},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			store := &fakeStore{}
			w := request(testHandler(t, store), tc.method, tc.path, tc.body, "bearer "+testToken)
			if w.Code != tc.status || store.called != tc.called {
				t.Fatalf("status=%d called=%s body=%s", w.Code, store.called, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Error("missing no-store")
			}
			if tc.method == "POST" {
				if store.novel.Title != "Book" || store.novel.Status != "ONGOING" {
					t.Errorf("unexpected defaults: %+v", store.novel)
				}
				if w.Header().Get("Location") != "/api/v2/novelinfo?id=7" {
					t.Error("missing Location")
				}
			}
			if store.called == "saveChapter" && (store.chapter.Title != "Chapter" || store.chapter.Content != "  body\n") {
				t.Errorf("content changed: %+v", store.chapter)
			}
			if store.called == "listNovels" && store.filter != (storage.NovelFilter{PlatformID: 2, AuthorID: 3, Limit: 5, Offset: 10}) {
				t.Errorf("filters: %+v", store.filter)
			}
			if store.called == "listChapters" && (store.id != 7 || store.limit != 3 || store.offset != 4) {
				t.Error("chapter pagination not forwarded")
			}
			if tc.status == 204 && w.Body.Len() != 0 {
				t.Error("204 must not contain body")
			}
			if strings.HasPrefix(store.called, "list") && strings.TrimSpace(w.Body.String()) != "[]" {
				t.Error("empty lists must be arrays")
			}
		})
	}
}

func TestAbandonedNovelStatus(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, called string
		status                     int
	}{
		{"POST", "/api/v2/novelinfo", `{"platformId":2,"title":"Book","status":"ABANDONED"}`, "createNovel", http.StatusCreated},
		{"PUT", "/api/v2/novelinfo?id=7", `{"platformId":2,"title":"Book","status":"ABANDONED"}`, "updateNovel", http.StatusOK},
		{"POST", "/api/v2/upload", `{"novel":{"platformId":2,"title":"Book","status":"ABANDONED"}}`, "upload", http.StatusCreated},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			store := &fakeStore{}
			w := request(testHandler(t, store), tc.method, tc.path, tc.body, "Bearer "+testToken)
			if w.Code != tc.status || store.called != tc.called || store.novel.Status != "ABANDONED" {
				t.Fatalf("status=%d called=%s novel=%+v body=%s", w.Code, store.called, store.novel, w.Body.String())
			}
			var result struct {
				Status string `json:"status"`
				Novel  struct {
					Status string `json:"status"`
				} `json:"novel"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Status != "ABANDONED" && result.Novel.Status != "ABANDONED" {
				t.Fatalf("response lost novel status: %s", w.Body.String())
			}
		})
	}
}

func TestNovelReferenceNames(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			store := &fakeStore{}
			path := "/api/v2/novelinfo"
			wantStatus := http.StatusCreated
			if method == http.MethodPut {
				path += "?id=7"
				wantStatus = http.StatusOK
			}
			w := request(testHandler(t, store), method, path,
				`{"platformId":2,"title":"Book","authorName":" 新作者 ","tagNames":[" 科幻 ","冒险","科幻"]}`, "Bearer "+testToken)
			if w.Code != wantStatus || store.novel.AuthorName == nil || *store.novel.AuthorName != "新作者" || store.novel.AuthorID != nil {
				t.Fatalf("status=%d novel=%+v body=%s", w.Code, store.novel, w.Body.String())
			}
			if strings.Join(store.novel.TagNames, ",") != "科幻,冒险" {
				t.Fatalf("tag names were not normalized: %v", store.novel.TagNames)
			}
		})
	}
	for _, body := range []string{
		`{"platformId":2,"title":"Book","authorId":3,"authorName":"新作者"}`,
		`{"platformId":2,"title":"Book","authorName":"bad\u0000name"}`,
		`{"platformId":2,"title":"Book","authorName":"` + strings.Repeat("作", 201) + `"}`,
		`{"platformId":2,"title":"Book","tagNames":[" "]}`,
		`{"platformId":2,"title":"Book","tagNames":["bad\u0000tag"]}`,
	} {
		store := &fakeStore{}
		w := request(testHandler(t, store), http.MethodPut, "/api/v2/novelinfo?id=7", body, "Bearer "+testToken)
		if w.Code != http.StatusBadRequest || store.called != "" {
			t.Fatalf("invalid references reached storage: status=%d called=%s body=%s", w.Code, store.called, w.Body.String())
		}
	}
}

func TestRejectedRequests(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/", "", 404},
		{"GET", "/api/v2/novelinfo/", "", 404},
		{"GET", "/health", "", 404},
		{"PATCH", "/api/v2/novelinfo", "", 405},
		{"POST", "/api/v2/content", "{}", 405},
		{"GET", "/api/v2/novelinfo?id=0", "", 400},
		{"GET", "/api/v2/novelinfo?id=2147483648", "", 400},
		{"GET", "/api/v2/novelinfo?id=1&id=2", "", 400},
		{"GET", "/api/v2/novelinfo?id=1&limit=2", "", 400},
		{"GET", "/api/v2/novelinfo?limit=101", "", 400},
		{"GET", "/api/v2/novelinfo?limit=0", "", 400},
		{"GET", "/api/v2/novelinfo?offset=-1", "", 400},
		{"GET", "/api/v2/novelinfo?platformId=", "", 400},
		{"GET", "/api/v2/novelinfo?unknown=1", "", 400},
		{"GET", "/api/v2/novelinfo?id=1;bad=2", "", 400},
		{"GET", "/api/v2/content", "", 400},
		{"GET", "/api/v2/content?novelId=1&order=0", "", 400},
		{"GET", "/api/v2/content?novelId=1&order=2&offset=1", "", 400},
		{"DELETE", "/api/v2/content?novelId=1", "", 400},
		{"DELETE", "/api/v2/novelinfo", "", 400},
		{"PUT", "/api/v2/novelinfo?id=1&platformId=2", "{}", 400},
		{"POST", "/api/v2/novelinfo?id=1", "{}", 400},
		{"POST", "/api/v2/novelinfo", "null", 400},
		{"POST", "/api/v2/novelinfo", "[]", 400},
		{"POST", "/api/v2/novelinfo", "{", 400},
		{"POST", "/api/v2/novelinfo", "{} {}", 400},
		{"POST", "/api/v2/novelinfo", `{"platformId":1,"title":"Book","categoryId":1}`, 400},
		{"POST", "/api/v2/novelinfo", `{"platformId":0,"title":"Book"}`, 400},
		{"POST", "/api/v2/novelinfo", `{"platformId":1,"title":" "}`, 400},
		{"POST", "/api/v2/novelinfo", `{"platformId":1,"title":"Book","status":"BAD"}`, 400},
		{"POST", "/api/v2/novelinfo", `{"platformId":1,"authorId":0,"title":"Book"}`, 400},
		{"PUT", "/api/v2/content?novelId=1", "{}", 400},
		{"PUT", "/api/v2/content", `{"novelId":1,"order":0,"title":"Chapter","content":"Body"}`, 400},
		{"PUT", "/api/v2/content", `{"novelId":1,"order":1,"title":"Chapter","content":" "}`, 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			store := &fakeStore{}
			w := request(testHandler(t, store), tc.method, tc.path, tc.body, "Bearer "+testToken)
			if w.Code != tc.status || store.called != "" {
				t.Fatalf("status=%d called=%s body=%s", w.Code, store.called, w.Body.String())
			}
			if tc.status == 405 && w.Header().Get("Allow") == "" {
				t.Error("missing Allow header")
			}
		})
	}
}

func TestChapterVolume(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		status      int
		volumeID    int32
	}{
		{"omitted", "", 200, 0},
		{"ungrouped", `,"volumeId":null`, 200, 0},
		{"grouped", `,"volumeId":5`, 200, 5},
		{"zero", `,"volumeId":0`, 400, 0},
		{"negative", `,"volumeId":-1`, 400, 0},
		{"overflow", `,"volumeId":2147483648`, 400, 0},
		{"fraction", `,"volumeId":1.5`, 400, 0},
		{"string", `,"volumeId":"5"`, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{}
			body := `{"novelId":7,"order":1,"title":"Chapter","content":"Body"` + tc.field + `}`
			w := request(testHandler(t, store), "PUT", "/api/v2/content", body, "Bearer "+testToken)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status != http.StatusOK {
				if store.called != "" {
					t.Fatal("invalid volume reached storage")
				}
				return
			}
			if store.called != "saveChapter" {
				t.Fatalf("called=%s", store.called)
			}
			var result storage.Chapter
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			for _, volumeID := range []*int32{store.chapter.VolumeID, result.VolumeID} {
				if tc.volumeID == 0 {
					if volumeID != nil {
						t.Fatal("ungrouped chapter has a volume")
					}
				} else if volumeID == nil || *volumeID != tc.volumeID {
					t.Fatal("volumeId was not preserved")
				}
			}
		})
	}
}

func TestBodyLimitsAndMediaType(t *testing.T) {
	handler := testHandler(t, nil)
	for _, tc := range []struct {
		body, mediaType string
		status          int
	}{
		{"{}", "text/plain", 415},
		{"{}", "", 415},
		{"{\"title\":\"\xff\"}", "application/json", 400},
		{strings.Repeat(" ", int(MaxBodyBytes)+1), "application/json", 413},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v2/novelinfo", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", tc.mediaType)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("expected %d got %d: %s", tc.status, w.Code, w.Body.String())
		}
	}
}

func TestStorageErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{storage.ErrNotFound, 404},
		{storage.ErrInvalidReference, 400},
		{storage.ErrConflict, 409},
		{storage.ErrImageMissing, 422},
		{context.DeadlineExceeded, 504},
		{errors.New("secret connection details"), 500},
	} {
		w := request(testHandler(t, &fakeStore{err: tc.err}), "GET", "/api/v2/novelinfo?id=1", "", "Bearer "+testToken)
		if w.Code != tc.status {
			t.Errorf("%v: status %d", tc.err, w.Code)
		}
		if strings.Contains(w.Body.String(), "secret") {
			t.Error("database error leaked")
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["error"] == "" {
			t.Error("invalid error envelope")
		}
	}
}

func TestPostgresTextValidation(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/v2/novelinfo", `{"platformId":1,"title":"Book\u0000"}`},
		{"POST", "/api/v2/novelinfo", `{"platformId":1,"title":"Book","summary":"\u0000"}`},
		{"PUT", "/api/v2/content", `{"novelId":1,"order":1,"title":"Chapter","content":"body\u0000"}`},
	} {
		store := &fakeStore{}
		w := request(testHandler(t, store), tc.method, tc.path, tc.body, "Bearer "+testToken)
		if w.Code != http.StatusBadRequest || store.called != "" {
			t.Fatalf("status=%d called=%s", w.Code, store.called)
		}
	}
}
