package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"albatextus/athub/internal/storage"
)

func (s *fakeStore) GetTOC(_ context.Context, novelID int32) (storage.TOC, error) {
	s.called, s.id = "getTOC", novelID
	return storage.TOC{NovelID: novelID}, s.err
}

type tocStore struct {
	fakeStore
	result storage.TOC
}

func (s *tocStore) GetTOC(_ context.Context, novelID int32) (storage.TOC, error) {
	s.called, s.id = "getTOC", novelID
	return s.result, s.err
}

func TestTOCResponse(t *testing.T) {
	volumeID := int32(5)
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	want := storage.TOC{NovelID: 7, WordCount: 12345,
		Volumes: []storage.Volume{{ID: volumeID, VolumeInput: storage.VolumeInput{NovelID: 7, Order: 1, Title: "Part 1"}, CreatedAt: now, UpdatedAt: now}},
		Chapters: []storage.ChapterSummary{
			{ID: 8, NovelID: 7, Order: 1, Title: "Prologue", UpdatedAt: now},
			{ID: 9, NovelID: 7, VolumeID: &volumeID, Order: 3, Title: "Chapter", UpdatedAt: now},
		}}
	store := &tocStore{result: want}
	w := request(testHandler(t, store), "GET", "/api/v2/toc?novelId=7", "", "Bearer "+testToken)
	if w.Code != http.StatusOK || store.called != "getTOC" || store.id != 7 {
		t.Fatalf("status=%d store=%+v body=%s", w.Code, store, w.Body.String())
	}
	var got storage.TOC
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("response: %+v, error: %v", got, err)
	}
	if strings.Contains(w.Body.String(), `"content"`) {
		t.Fatal("TOC must not include chapter content")
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatal("missing response headers")
	}
	// Nil store slices must still serialize as arrays, not null.
	w = request(testHandler(t, &fakeStore{}), "GET", "/api/v2/toc?novelId=7", "", "Bearer "+testToken)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"novelId":7,"wordCount":0,"volumes":[],"chapters":[]}` {
		t.Fatalf("empty TOC: %d %s", w.Code, w.Body.String())
	}
}

func TestTOCRejectedRequests(t *testing.T) {
	for _, path := range []string{
		"/api/v2/toc", "/api/v2/toc?novelId=", "/api/v2/toc?novelId=0", "/api/v2/toc?novelId=-1",
		"/api/v2/toc?novelId=2147483648", "/api/v2/toc?novelId=1.5", "/api/v2/toc?novelId=abc",
		"/api/v2/toc?novelId=1&novelId=2", "/api/v2/toc?novelId=1&limit=20",
		"/api/v2/toc?novelId=1&offset=0", "/api/v2/toc?novelId=1&order=1",
		"/api/v2/toc?novelId=1&volumeId=2", "/api/v2/toc?novelId=%zz",
	} {
		t.Run(path, func(t *testing.T) {
			store := &fakeStore{}
			w := request(testHandler(t, store), "GET", path, "", "Bearer "+testToken)
			if w.Code != 400 || store.called != "" {
				t.Fatalf("status=%d called=%s body=%s", w.Code, store.called, w.Body.String())
			}
		})
	}
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"} {
		store := &fakeStore{}
		w := request(testHandler(t, store), method, "/api/v2/toc?novelId=7", "", "Bearer "+testToken)
		if w.Code != 405 || w.Header().Get("Allow") != "GET" || store.called != "" {
			t.Fatalf("%s: status=%d Allow=%s called=%s", method, w.Code, w.Header().Get("Allow"), store.called)
		}
	}
	store := &fakeStore{}
	w := request(testHandler(t, store), "GET", "/api/v2/toc?novelId=7", "", "")
	if w.Code != 401 || store.called != "" {
		t.Fatal("unauthenticated TOC request reached storage")
	}
}

func TestTOCStorageErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{storage.ErrNotFound, 404},
		{context.DeadlineExceeded, 504},
		{errors.New("private database details"), 500},
	} {
		w := request(testHandler(t, &fakeStore{err: tc.err}), "GET", "/api/v2/toc?novelId=7", "", "Bearer "+testToken)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private database details") {
			t.Fatalf("error=%v status=%d body=%s", tc.err, w.Code, w.Body.String())
		}
	}
}
