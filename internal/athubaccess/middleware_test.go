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

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
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
func accessFor(t *testing.T, s *accessStore) *Access {
	t.Helper()
	a, err := New(s, ":8003", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return a
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &accessStore{identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: tc.active}, resolveErr: tc.resolveErr, beginErr: tc.beginErr}
			dispatched := false
			handler := accessFor(t, s).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched = true
				if s.call.ID == uuid.Nil {
					t.Fatal("content dispatched before audit started")
				}
				w.WriteHeader(204)
			}))
			r := httptest.NewRequest("GET", "/api/v2/content?token=test-query-token", nil)
			r.Host = "attacker.example:9999"
			r.Header.Set("Authorization", tc.auth)
			r.Header.Set("X-Forwarded-Port", "9999")
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
				if s.call.Port != 8003 || s.call.Path != "/api/v2/content" {
					t.Fatalf("unsafe audit labels: %+v", s.call)
				}
			}
			if tc.status == 401 && w.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing bearer challenge")
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
		handler := accessFor(t, s).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
