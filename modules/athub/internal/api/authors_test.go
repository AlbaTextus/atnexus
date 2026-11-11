package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"albatextus/athub/internal/storage"
)

func (s *fakeStore) UpdateAuthorName(_ context.Context, id int32, name *string) (storage.Author, error) {
	s.called, s.id = "updateAuthorName", id
	return storage.Author{ID: id, AuthorInput: storage.AuthorInput{PlatformID: 2, Name: name}}, s.err
}

func TestAuthorNames(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
		name               string
	}{
		{"POST", "/api/v2/authors", `{"platformId":2,"name":"  真实笔名  "}`, 201, "真实笔名"},
		{"POST", "/api/v2/authors", `{"platformId":2}`, 201, ""},
		{"PUT", "/api/v2/authors?id=3", `{"name":" 新笔名 "}`, 200, "新笔名"},
		{"PUT", "/api/v2/authors?id=3", `{"name":null}`, 200, ""},
		{"PUT", "/api/v2/authors?id=3", `{"name":" "}`, 200, ""},
		{"PUT", "/api/v2/authors", `{"name":"笔名"}`, 400, ""},
		{"PUT", "/api/v2/authors?id=3", `{"platformId":4,"name":"笔名"}`, 400, ""},
		{"POST", "/api/v2/authors", `{"platformId":2,"name":"a\u0000b"}`, 400, ""},
		{"PUT", "/api/v2/authors?id=3", `{"name":"a\u0000b"}`, 400, ""},
		{"PUT", "/api/v2/authors?id=3", `{"name":"` + strings.Repeat("字", 201) + `"}`, 400, ""},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			st := &fakeStore{}
			w := request(testHandler(t, st), tc.method, tc.path, tc.body, "Bearer "+testToken)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if tc.status >= 400 {
				if st.called != "" {
					t.Fatal("invalid author reached storage")
				}
				return
			}
			var author storage.Author
			if err := json.Unmarshal(w.Body.Bytes(), &author); err != nil {
				t.Fatal(err)
			}
			if tc.name == "" && author.Name != nil || tc.name != "" && (author.Name == nil || *author.Name != tc.name) {
				t.Fatalf("name=%v want=%q", author.Name, tc.name)
			}
		})
	}
	w := request(testHandler(t, &fakeStore{err: storage.ErrNotFound}), "PUT", "/api/v2/authors?id=999", `{"name":"笔名"}`, "Bearer "+testToken)
	if w.Code != 404 {
		t.Fatalf("missing author status=%d", w.Code)
	}
}
