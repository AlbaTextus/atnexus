package api

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"albatextus/athub/internal/storage"
)

func (s *fakeStore) GetTag(_ context.Context, id int32) (storage.Tag, error) {
	s.called, s.id = "getTag", id
	return storage.Tag{ID: id}, s.err
}
func (s *fakeStore) ListTags(_ context.Context, limit, offset int32) ([]storage.Tag, error) {
	s.called, s.limit, s.offset = "listTags", limit, offset
	return nil, s.err
}
func (s *fakeStore) CreateTag(_ context.Context, input storage.TagInput) (storage.Tag, error) {
	s.called, s.novel.Title = "createTag", input.Name
	return storage.Tag{ID: 9, TagInput: input}, s.err
}
func (s *fakeStore) UpdateTag(_ context.Context, id int32, input storage.TagInput) (storage.Tag, error) {
	s.called, s.id, s.novel.Title = "updateTag", id, input.Name
	return storage.Tag{ID: id, TagInput: input}, s.err
}
func (s *fakeStore) DeleteTag(_ context.Context, id int32) error {
	s.called, s.id = "deleteTag", id
	return s.err
}

func TestTagRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path, body, called string
		status                     int
	}{
		{"GET", "/api/v2/tags?id=9", "", "getTag", 200},
		{"GET", "/api/v2/tags?limit=3&offset=4", "", "listTags", 200},
		{"POST", "/api/v2/tags", `{"name":"  奇幻  "}`, "createTag", 201},
		{"PUT", "/api/v2/tags?id=9", `{"name":"  奇幻  "}`, "updateTag", 200},
		{"DELETE", "/api/v2/tags?id=9", "", "deleteTag", 204},
		{"PATCH", "/api/v2/tags", "", "", 405},
		{"GET", "/api/v2/tags?id=9&limit=2", "", "", 400},
		{"GET", "/api/v2/tags?id=0", "", "", 400},
		{"GET", "/api/v2/tags?limit=101", "", "", 400},
		{"GET", "/api/v2/tags?id=1&id=2", "", "", 400},
		{"POST", "/api/v2/tags?id=9", `{"name":"奇幻"}`, "", 400},
		{"PUT", "/api/v2/tags", `{"name":"奇幻"}`, "", 400},
		{"DELETE", "/api/v2/tags", "", "", 400},
		{"POST", "/api/v2/tags", `{"name":" "}`, "", 400},
		{"POST", "/api/v2/tags", `{"name":"奇\u0000幻"}`, "", 400},
		{"POST", "/api/v2/tags", `{"name":"奇幻","id":1}`, "", 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			store := &fakeStore{}
			w := request(testHandler(t, store), tc.method, tc.path, tc.body, "Bearer "+testToken)
			if w.Code != tc.status || store.called != tc.called {
				t.Fatalf("status=%d called=%s body=%s", w.Code, store.called, w.Body)
			}
			if tc.called == "createTag" && w.Header().Get("Location") != "/api/v2/tags?id=9" {
				t.Fatal("missing Location")
			}
			if tc.called == "createTag" || tc.called == "updateTag" {
				if store.novel.Title != "奇幻" {
					t.Fatal("tag name was not trimmed")
				}
			}
			if tc.called == "listTags" && (store.limit != 3 || store.offset != 4 || strings.TrimSpace(w.Body.String()) != "[]") {
				t.Fatal("incorrect empty list or pagination")
			}
		})
	}
	for _, tc := range []struct {
		err    error
		status int
	}{
		{storage.ErrNotFound, 404}, {storage.ErrConflict, 409}, {storage.ErrTagInUse, 409},
	} {
		w := request(testHandler(t, &fakeStore{err: tc.err}), "DELETE", "/api/v2/tags?id=9", "", "Bearer "+testToken)
		if w.Code != tc.status {
			t.Fatalf("error %v: status=%d", tc.err, w.Code)
		}
	}
	if w := request(testHandler(t, nil), "GET", "/api/v2/tags", "", ""); w.Code != 401 {
		t.Fatal("tags must require authentication")
	}
}

func TestNovelTagReferences(t *testing.T) {
	for _, tc := range []struct{ method, path, prefix, suffix string }{
		{"POST", "/api/v2/novelinfo", "", ""},
		{"PUT", "/api/v2/novelinfo?id=7", "", ""},
		{"POST", "/api/v2/upload", `{"novel":`, "}"},
	} {
		for _, ids := range []string{`[3,9]`, `[]`, `null`, `[0]`, `[-1]`, `[3,3]`, `["3"]`, `[1.5]`, `[2147483648]`} {
			t.Run(tc.method+tc.path+ids, func(t *testing.T) {
				store := &fakeStore{}
				body := tc.prefix + `{"platformId":1,"title":"Book","tagIds":` + ids + `}` + tc.suffix
				w := request(testHandler(t, store), tc.method, tc.path, body, "Bearer "+testToken)
				valid := ids == `[3,9]` || ids == `[]` || ids == `null`
				if !valid {
					if w.Code != 400 || store.called != "" {
						t.Fatalf("invalid references reached storage: %d %s", w.Code, w.Body)
					}
					return
				}
				var result storage.Novel
				if tc.path == "/api/v2/upload" {
					var upload storage.UploadResult
					if err := json.Unmarshal(w.Body.Bytes(), &upload); err != nil {
						t.Fatal(err)
					}
					result = upload.Novel
				} else if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				want := []int32{}
				if ids == `[3,9]` {
					want = []int32{3, 9}
				}
				if w.Code >= 400 || !reflect.DeepEqual(result.TagIDs, want) {
					t.Fatalf("tagIds=%v status=%d body=%s", result.TagIDs, w.Code, w.Body)
				}
			})
		}
	}
	store := &fakeStore{}
	w := request(testHandler(t, store), "GET", "/api/v2/novelinfo?tagId=9&platformId=2&limit=3", "", "Bearer "+testToken)
	if w.Code != 200 || store.filter.TagID != 9 || store.filter.PlatformID != 2 || store.filter.Limit != 3 {
		t.Fatalf("filter not forwarded: %+v status=%d", store.filter, w.Code)
	}
	for _, path := range []string{"/api/v2/novelinfo?tagId=0", "/api/v2/novelinfo?id=1&tagId=9"} {
		if w := request(testHandler(t, nil), "GET", path, "", "Bearer "+testToken); w.Code != 400 {
			t.Fatalf("%s: status=%d", path, w.Code)
		}
	}
}
