package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
)

type cachedStore struct {
	fakeStore
	reads   atomic.Int32
	title   atomic.Value
	fail    atomic.Bool
	started chan struct{}
	release chan struct{}
}

func (s *cachedStore) GetNovel(ctx context.Context, id int32) (storage.Novel, error) {
	count := s.reads.Add(1)
	title, _ := s.title.Load().(string)
	if count == 1 && s.started != nil {
		close(s.started)
		select {
		case <-s.release:
		case <-ctx.Done():
			return storage.Novel{}, ctx.Err()
		}
	}
	if s.fail.Load() {
		return storage.Novel{}, errors.New("database unavailable")
	}
	return storage.Novel{ID: id, NovelInput: storage.NovelInput{Title: title}}, nil
}
func (s *cachedStore) CreateNovel(_ context.Context, n storage.NovelInput) (storage.Novel, error) {
	s.title.Store(n.Title)
	return storage.Novel{ID: 1, NovelInput: n}, nil
}

func TestReadCacheStillAuthenticatesAndInvalidates(t *testing.T) {
	st := &cachedStore{}
	st.title.Store("before")
	var admitted atomic.Int32
	var denied atomic.Bool
	handler, err := NewWithMiddleware(st, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			admitted.Add(1)
			if denied.Load() {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		w := request(handler, "GET", "/api/v2/novelinfo?id=1", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "before") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read status=%d body=%s headers=%v", w.Code, w.Body, w.Header())
		}
	}
	if st.reads.Load() != 1 || admitted.Load() != 2 {
		t.Fatalf("cache bypassed accounting or missed: reads=%d admissions=%d", st.reads.Load(), admitted.Load())
	}
	denied.Store(true)
	if w := request(handler, "GET", "/api/v2/novelinfo?id=1", ""); w.Code != 401 {
		t.Fatal("cache bypassed revoked authentication")
	}
	denied.Store(false)
	w := request(handler, "POST", "/api/v2/novelinfo", `{"platformId":1,"title":"after","status":"ONGOING"}`)
	if w.Code != 201 {
		t.Fatalf("mutation %d %s", w.Code, w.Body)
	}
	w = request(handler, "GET", "/api/v2/novelinfo?id=1", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "after") || st.reads.Load() != 2 {
		t.Fatalf("mutation left stale cache: %d %s reads=%d", w.Code, w.Body, st.reads.Load())
	}
}

func TestReadCacheCoalescesAndRejectsStaleFill(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent readers", true: "mutation during fill"}[mutate], func(t *testing.T) {
			st := &cachedStore{started: make(chan struct{}), release: make(chan struct{})}
			st.title.Store("before")
			a := &API{store: st, logger: nil, cache: newReadCache()}
			handler := http.HandlerFunc(a.serveCachedContent)
			var readers sync.WaitGroup
			readers.Add(1)
			go func() {
				defer readers.Done()
				request(handler, "GET", "/api/v2/novelinfo?id=1", "")
			}()
			<-st.started
			if mutate {
				request(handler, "POST", "/api/v2/novelinfo", `{"platformId":1,"title":"after","status":"ONGOING"}`)
			} else {
				for i := 0; i < 12; i++ {
					readers.Add(1)
					go func() {
						defer readers.Done()
						request(handler, "GET", "/api/v2/novelinfo?id=1", "")
					}()
				}
			}
			close(st.release)
			readers.Wait()
			w := request(handler, "GET", "/api/v2/novelinfo?id=1", "")
			wantReads, title := int32(1), "before"
			if mutate {
				wantReads, title = 2, "after"
			}
			if st.reads.Load() != wantReads || !strings.Contains(w.Body.String(), title) {
				t.Fatalf("reads=%d want=%d body=%s", st.reads.Load(), wantReads, w.Body)
			}
			a.cache.mu.Lock()
			for key, entry := range a.cache.entries {
				entry.expires = time.Now().Add(-time.Second)
				a.cache.entries[key] = entry
			}
			a.cache.mu.Unlock()
			request(handler, "GET", "/api/v2/novelinfo?id=1", "")
			if st.reads.Load() != wantReads+1 {
				t.Fatal("expired entry did not reload")
			}
		})
	}
}

func TestReadCacheBoundsAndErrors(t *testing.T) {
	st := &cachedStore{}
	st.title.Store("ok")
	// NewWithMiddleware supplies the error logger, including the error path.
	handler, _ := NewWithMiddleware(st, func(next http.Handler) http.Handler { return next }, nil)
	st.fail.Store(true)
	if w := request(handler, "GET", "/api/v2/novelinfo?id=1", ""); w.Code != 500 {
		t.Fatalf("error status=%d", w.Code)
	}
	st.fail.Store(false)
	if w := request(handler, "GET", "/api/v2/novelinfo?id=1", ""); w.Code != 200 || st.reads.Load() != 2 {
		t.Fatal("error response was cached")
	}
	st.title.Store(strings.Repeat("x", readCacheItemBytes+1))
	for i := 0; i < 2; i++ {
		request(handler, "GET", "/api/v2/novelinfo?id=2", "")
	}
	if st.reads.Load() != 4 {
		t.Fatal("oversized response was cached")
	}
}
