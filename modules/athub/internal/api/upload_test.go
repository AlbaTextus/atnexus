package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

func (s *fakeStore) GetPlatform(_ context.Context, id int32) (storage.Platform, error) {
	s.called, s.id = "getPlatform", id
	return storage.Platform{ID: id}, s.err
}
func (s *fakeStore) ListPlatforms(_ context.Context, limit, offset int32) ([]storage.Platform, error) {
	s.called, s.limit, s.offset = "listPlatforms", limit, offset
	return nil, s.err
}
func (s *fakeStore) CreatePlatform(_ context.Context, input storage.PlatformInput) (storage.Platform, error) {
	s.called = "createPlatform"
	return storage.Platform{ID: 2, PlatformInput: input}, s.err
}
func (s *fakeStore) GetAuthor(_ context.Context, id int32) (storage.Author, error) {
	s.called, s.id = "getAuthor", id
	return storage.Author{ID: id}, s.err
}
func (s *fakeStore) ListAuthors(_ context.Context, platformID, limit, offset int32) ([]storage.Author, error) {
	s.called, s.id, s.limit, s.offset = "listAuthors", platformID, limit, offset
	return nil, s.err
}
func (s *fakeStore) CreateAuthor(_ context.Context, input storage.AuthorInput) (storage.Author, error) {
	s.called = "createAuthor"
	return storage.Author{ID: 3, AuthorInput: input}, s.err
}
func (s *fakeStore) GetVolume(_ context.Context, novelID, order int32) (storage.Volume, error) {
	s.called, s.id, s.order = "getVolume", novelID, order
	return storage.Volume{ID: 5, VolumeInput: storage.VolumeInput{NovelID: novelID, Order: order}}, s.err
}
func (s *fakeStore) ListVolumes(_ context.Context, novelID, limit, offset int32) ([]storage.Volume, error) {
	s.called, s.id, s.limit, s.offset = "listVolumes", novelID, limit, offset
	return nil, s.err
}
func (s *fakeStore) SaveVolume(_ context.Context, input storage.VolumeInput) (storage.Volume, error) {
	s.called = "saveVolume"
	return storage.Volume{ID: 5, VolumeInput: input}, s.err
}
func (s *fakeStore) DeleteVolume(_ context.Context, novelID, order int32) error {
	s.called, s.id, s.order = "deleteVolume", novelID, order
	return s.err
}
func (s *fakeStore) Upload(_ context.Context, input storage.UploadInput) (storage.UploadResult, error) {
	s.called = "upload"
	if input.Novel != nil {
		s.novel = *input.Novel
	}
	return storage.UploadResult{Novel: storage.Novel{ID: 7, NovelInput: s.novel},
		Volumes: []storage.Volume{}, Chapters: []storage.UploadedChapter{}, ChapterCount: len(input.Chapters)}, s.err
}

func TestCatalogAndVolumeRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, called, location string
		status                               int
	}{
		{"GET", "/api/v2/platforms?id=2", "", "getPlatform", "", 200},
		{"GET", "/api/v2/platforms?limit=3&offset=4", "", "listPlatforms", "", 200},
		{"POST", "/api/v2/platforms", `{"name":" Local ","url":null}`, "createPlatform", "/api/v2/platforms?id=2", 201},
		{"GET", "/api/v2/authors?id=3", "", "getAuthor", "", 200},
		{"GET", "/api/v2/authors?platformId=2&limit=3&offset=4", "", "listAuthors", "", 200},
		{"POST", "/api/v2/authors", `{"platformId":2}`, "createAuthor", "/api/v2/authors?id=3", 201},
		{"GET", "/api/v2/volumes?novelId=7&order=2", "", "getVolume", "", 200},
		{"GET", "/api/v2/volumes?novelId=7&limit=3&offset=4", "", "listVolumes", "", 200},
		{"PUT", "/api/v2/volumes", `{"novelId":7,"order":2,"title":" Part 2 "}`, "saveVolume", "", 200},
		{"DELETE", "/api/v2/volumes?novelId=7&order=2", "", "deleteVolume", "", 204},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			store := &fakeStore{}
			w := request(testHandler(t, store), tc.method, tc.path, tc.body, "Bearer "+testToken)
			if w.Code != tc.status || store.called != tc.called || w.Header().Get("Location") != tc.location {
				t.Fatalf("status=%d called=%s body=%s", w.Code, store.called, w.Body.String())
			}
			if strings.HasPrefix(store.called, "list") {
				if strings.TrimSpace(w.Body.String()) != "[]" || store.limit != 3 || store.offset != 4 {
					t.Fatal("list pagination/empty response mismatch")
				}
			}
			if store.called == "listAuthors" && store.id != 2 {
				t.Fatal("author platform filter lost")
			}
			if (store.called == "getVolume" || store.called == "deleteVolume") && (store.id != 7 || store.order != 2) {
				t.Fatal("novel/order lost")
			}
			if store.called == "saveVolume" && !strings.Contains(w.Body.String(), `"title":"Part 2"`) {
				t.Fatal("volume title was not trimmed")
			}
		})
	}
}

func TestNewRouteRejections(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/v2/platforms?id=2&limit=1", ""},
		{"GET", "/api/v2/platforms?unknown=1", ""},
		{"POST", "/api/v2/platforms?id=2", `{"name":"Local"}`},
		{"POST", "/api/v2/platforms", `{"name":" "}`},
		{"POST", "/api/v2/platforms", `{"name":"Local","url":"\u0000"}`},
		{"GET", "/api/v2/authors?id=3&platformId=2", ""},
		{"GET", "/api/v2/authors?platformId=0", ""},
		{"POST", "/api/v2/authors?limit=1", `{"platformId":2}`},
		{"POST", "/api/v2/authors", `{"platformId":0}`},
		{"POST", "/api/v2/authors", `{"platformId":2,"unknown":"unsupported"}`},
		{"GET", "/api/v2/volumes", ""},
		{"GET", "/api/v2/volumes?novelId=7&order=1&limit=1", ""},
		{"DELETE", "/api/v2/volumes?novelId=7", ""},
		{"PUT", "/api/v2/volumes?novelId=7", `{}`},
		{"PUT", "/api/v2/volumes", `{"novelId":7,"order":0,"title":"Part"}`},
		{"PUT", "/api/v2/volumes", `{"novelId":7,"order":1,"title":"\u0000"}`},
		{"POST", "/api/v2/upload?novelId=7", `{}`},
		{"POST", "/api/v2/upload", `{}`},
		{"POST", "/api/v2/upload", `{"novelId":0}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"novel":{"platformId":2,"title":"Book"}}`},
		{"POST", "/api/v2/upload", `{"novelId":7}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"authorName":"作者","chapters":[{"order":1,"title":"C","content":"B"}]}`},
		{"POST", "/api/v2/upload", `{"novel":{"platformId":2,"title":"Book","authorId":3},"authorName":"作者"}`},
		{"POST", "/api/v2/upload", `{"novel":{"platformId":2,"title":"Book"},"authorName":"bad\u0000name"}`},
		{"POST", "/api/v2/upload", `{"novel":{"platformId":2,"title":"Book"},"authorName":"` + strings.Repeat("作", 201) + `"}`},
		{"POST", "/api/v2/upload", `{"novel":{"platformId":2,"title":" "}}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"volumes":[{"order":1,"title":"P"},{"order":1,"title":"P"}]}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"volumes":[{"order":1,"title":" "}]}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"chapters":[{"order":1,"title":"C","content":"B","volumeOrder":0}]}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"chapters":[{"order":1,"title":"C","content":" "}]}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"chapters":[{"order":1,"title":"C","content":"B"},{"order":1,"title":"C","content":"B"}]}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"chapters":[{"order":1,"title":"C","content":"B","volumeId":5}]}`},
		{"POST", "/api/v2/upload", `{"novelId":7,"chapters":[null]}`},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			store := &fakeStore{}
			w := request(testHandler(t, store), tc.method, tc.path, tc.body, "Bearer "+testToken)
			if w.Code != 400 || store.called != "" {
				t.Fatalf("status=%d called=%s body=%s", w.Code, store.called, w.Body.String())
			}
		})
	}
	for _, path := range []string{"/api/v2/platforms", "/api/v2/authors", "/api/v2/volumes", "/api/v2/upload"} {
		if w := request(testHandler(t, nil), "GET", path, "", ""); w.Code != 401 {
			t.Fatalf("%s did not require authentication", path)
		}
		if w := request(testHandler(t, nil), "PATCH", path, "", "Bearer "+testToken); w.Code != 405 || w.Header().Get("Allow") == "" {
			t.Fatalf("%s allowed unsupported method", path)
		}
	}
}

func TestUploadValidationAndResponse(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"novel":{"platformId":2,"title":" Book "},"volumes":[{"order":1,"title":" Part "}],"chapters":[{"order":1,"volumeOrder":1,"title":" C ","content":" B\n"}]}`, 201},
		{`{"novelId":7,"chapters":[{"order":2,"title":"C","content":"B"}]}`, 200},
		{`{"novel":{"platformId":2,"title":"Book"}}`, 201},
	} {
		store := &fakeStore{}
		w := request(testHandler(t, store), "POST", "/api/v2/upload", tc.body, "Bearer "+testToken)
		if w.Code != tc.status || store.called != "upload" {
			t.Fatalf("status=%d called=%s body=%s", w.Code, store.called, w.Body.String())
		}
		if tc.status == 201 && (store.novel.Title != "Book" || store.novel.Status != "ONGOING" || w.Header().Get("Location") != "/api/v2/novelinfo?id=7") {
			t.Fatalf("invalid new novel: %+v", store.novel)
		}
	}
	input := storage.UploadInput{Novel: &storage.NovelInput{PlatformID: 2, Title: "Book"},
		Volumes:  []storage.UploadVolume{{Order: 1, Title: " Part "}},
		Chapters: []storage.UploadChapter{{Order: 1, Title: " C ", Content: " body\n"}}}
	if err := validateUpload(&input); err != nil || input.Volumes[0].Title != "Part" || input.Chapters[0].Title != "C" || input.Chapters[0].Content != " body\n" {
		t.Fatalf("validation changed body or failed: %+v %v", input, err)
	}
	for _, size := range []struct{ volumes, chapters int }{{201, 0}, {0, 1001}} {
		input := storage.UploadInput{Novel: &storage.NovelInput{PlatformID: 2, Title: "Book"},
			Volumes: make([]storage.UploadVolume, size.volumes), Chapters: make([]storage.UploadChapter, size.chapters)}
		data, _ := json.Marshal(input)
		store := &fakeStore{}
		if w := request(testHandler(t, store), "POST", "/api/v2/upload", string(data), "Bearer "+testToken); w.Code != 400 || store.called != "" {
			t.Fatalf("accepted oversized batch: %d", w.Code)
		}
	}
	for _, tc := range []struct {
		body, media string
		status      int
	}{
		{`{}`, "multipart/form-data; boundary=x", 415},
		{strings.Repeat(" ", int(MaxBodyBytes)+1), "application/json", 413},
	} {
		r := httptest.NewRequest("POST", "/api/v2/upload", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set("Content-Type", tc.media)
		w := httptest.NewRecorder()
		testHandler(t, nil).ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("upload media/limit: status=%d", w.Code)
		}
	}
	w := request(testHandler(t, &fakeStore{err: storage.ErrVolumeNotEmpty}), "DELETE", "/api/v2/volumes?novelId=7&order=1", "", "Bearer "+testToken)
	if w.Code != 409 {
		t.Fatalf("nonempty volume: %d", w.Code)
	}
}

func TestUploadAuthorName(t *testing.T) {
	for _, name := range []string{"  EPUB 作者  ", "   "} {
		input := storage.UploadInput{Novel: &storage.NovelInput{PlatformID: 2, Title: "Book"}, AuthorName: &name}
		if err := validateUpload(&input); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(name) == "" {
			if input.AuthorName != nil {
				t.Fatal("blank name should not create an author")
			}
		} else if input.AuthorName == nil || *input.AuthorName != "EPUB 作者" {
			t.Fatal("author name was lost or not trimmed")
		}
	}
	w := request(testHandler(t, &fakeStore{}), "POST", "/api/v2/upload",
		`{"novel":{"platformId":2,"title":"Book"},"authorName":"EPUB 作者"}`, "Bearer "+testToken)
	if w.Code != 201 {
		t.Fatalf("upload rejected authorName: %d %s", w.Code, w.Body.String())
	}
}
