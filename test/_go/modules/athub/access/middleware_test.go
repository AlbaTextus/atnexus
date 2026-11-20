package athubaccess

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

type accessStore struct {
	store.ATHubStore
	identity                        store.ATHubIdentity
	resolveErr, beginErr, finishErr error
	hash                            string
	call                            store.ATHubCall
	status                          int
	finished                        bool
}

func (s *accessStore) UserByID(_ context.Context, id uuid.UUID) (store.User, error) {
	name := "测试贡献者"
	return store.User{ID: id, NumericID: 1, DisplayName: &name, IsActive: true}, nil
}

func (s *accessStore) ResolveATHubKey(_ context.Context, hash string) (store.ATHubIdentity, error) {
	s.hash = hash
	return s.identity, s.resolveErr
}
func (s *accessStore) BeginATHubCall(_ context.Context, call store.ATHubCall) error {
	s.call = call
	return s.beginErr
}
func (s *accessStore) FinishATHubCall(ctx context.Context, _ uuid.UUID, status int, _, _ int64) error {
	if ctx.Err() != nil {
		panic("audit context was cancelled")
	}
	s.status = status
	s.finished = true
	return s.finishErr
}
func accessFor(s *accessStore) *Access {
	return New(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestAuthenticationAndAccounting(t *testing.T) {
	token := "ath_" + strings.Repeat("a", 43)
	for _, tc := range []struct {
		name, auth           string
		active               bool
		resolveErr, beginErr error
		status               int
		duplicate            bool
	}{
		{"valid", "Bearer " + token, true, nil, nil, 204, false},
		{"case insensitive scheme", "bearer " + token, true, nil, nil, 204, false},
		{"missing", "", false, nil, nil, 401, false},
		{"global token", "Bearer " + strings.Repeat("t", 64), false, nil, nil, 401, false},
		{"unknown", "Bearer " + token, false, store.ErrNotFound, nil, 401, false},
		{"revoked expired or disabled", "Bearer " + token, false, nil, nil, 401, false},
		{"duplicate header", "Bearer " + token, true, nil, nil, 401, true},
		{"trailing space", "Bearer " + token + " ", true, nil, nil, 401, false},
		{"double space", "Bearer  " + token, true, nil, nil, 401, false},
		{"lookup unavailable", "Bearer " + token, true, errors.New("offline"), nil, 503, false},
		{"recording unavailable", "Bearer " + token, true, nil, errors.New("offline"), 503, false},
		{"admin disabled between lookup and dispatch", "Bearer " + token, true, nil, store.ErrATHubAccessDenied, 401, false},
		{"total call limit reached", "Bearer " + token, true, nil, store.ErrATHubCallLimit, 429, false},
		{"operation forbidden", "Bearer " + token, true, nil, store.ErrATHubPermissionDenied, 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &accessStore{identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: tc.active}, resolveErr: tc.resolveErr, beginErr: tc.beginErr}
			dispatched := false
			handler := accessFor(s).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched = true
				if s.call.ID == uuid.Nil {
					t.Fatal("content dispatched before audit started")
				}
				w.WriteHeader(204)
			}))
			r := httptest.NewRequest("GET", "/api/v2/content?token=test-query-token", nil)
			r.Header.Set("Authorization", tc.auth)
			if tc.duplicate {
				r.Header.Add("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || dispatched != (tc.status == 204) {
				t.Fatalf("status=%d dispatched=%v", w.Code, dispatched)
			}
			if tc.status != 503 {
				if !s.finished || s.status != tc.status {
					t.Fatalf("completion missing: %+v", s)
				}
				if s.call.Path != "/api/v2/content" {
					t.Fatalf("unsafe audit labels: %+v", s.call)
				}
			}
			if tc.status == 401 && w.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing bearer challenge")
			}
			if tc.name == "total call limit reached" && !strings.Contains(w.Body.String(), "api_key_total_call_limit_reached") {
				t.Fatal("missing total call limit error")
			}
			if tc.status == 204 && (s.hash != security.HashToken(token) || s.call.UserID == nil || !s.call.Authenticated) {
				t.Fatal("user or hash association missing")
			}
		})
	}
}

func TestCompletionSurvivesCancellationAndPanic(t *testing.T) {
	for _, panics := range []bool{false, true} {
		s := &accessStore{identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: true}}
		ctx, cancel := context.WithCancel(context.Background())
		handler := accessFor(s).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cancel()
			if panics {
				panic("private panic data")
			}
			_, _ = w.Write([]byte("ok"))
		}))
		r := httptest.NewRequest("GET", "/api/v2/secret-unbounded-path", nil).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer ath_"+strings.Repeat("a", 43))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		want := 200
		if panics {
			want = 500
		}
		if !s.finished || s.status != want || w.Code != want {
			t.Fatalf("completion=%v status=%d response=%d", s.finished, s.status, w.Code)
		}
		if s.call.Path != "/api/v2/{unknown}" {
			t.Fatal("unbounded path entered accounting")
		}
	}
}

func TestTagsUseAuthenticatedAccounting(t *testing.T) {
	s := &accessStore{identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: true}}
	dispatched := false
	handler := accessFor(s).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched = true
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest("DELETE", "/api/v2/tags?id=3", nil)
	r.Header.Set("Authorization", "Bearer ath_"+strings.Repeat("a", 43))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !dispatched || w.Code != http.StatusNoContent || !s.finished || s.call.Path != "/api/v2/tags" || !s.call.Authenticated {
		t.Fatalf("tags authentication or accounting failed: status=%d call=%+v", w.Code, s.call)
	}
}
