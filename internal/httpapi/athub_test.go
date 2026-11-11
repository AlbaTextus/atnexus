package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type keyStore struct {
	store.Store
	store.ATHubStore
	user        store.User
	key         store.ATHubKey
	queriedUser uuid.UUID
	from, until time.Time
	bucket      int
}

func (s *keyStore) SessionByHash(context.Context, string) (store.Session, error) {
	return store.Session{UserID: s.user.ID, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (s *keyStore) UserByID(context.Context, uuid.UUID) (store.User, error) { return s.user, nil }
func (s *keyStore) CreateATHubKey(_ context.Context, key store.ATHubKey) (store.ATHubKey, error) {
	s.key = key
	return key, nil
}
func (s *keyStore) EnsureATHubKey(_ context.Context, key store.ATHubKey) (store.ATHubKey, bool, error) {
	if s.key.ID != uuid.Nil {
		return s.key, false, nil
	}
	s.key = key
	return key, true, nil
}
func (s *keyStore) ResetATHubKey(_ context.Context, key store.ATHubKey) (store.ATHubKey, error) {
	s.key = key
	return key, nil
}
func (s *keyStore) ListATHubKeys(_ context.Context, user uuid.UUID) ([]store.ATHubKey, error) {
	s.queriedUser = user
	return []store.ATHubKey{s.key}, nil
}
func (s *keyStore) RevokeATHubKey(_ context.Context, user, id uuid.UUID) error {
	s.queriedUser = user
	if s.key.UserID != user || s.key.ID != id {
		return store.ErrNotFound
	}
	now := time.Now()
	s.key.RevokedAt = &now
	return nil
}
func (s *keyStore) ATHubUsage(_ context.Context, user uuid.UUID) (store.ATHubUsage, error) {
	s.queriedUser = user
	return store.ATHubUsage{}, nil
}
func (s *keyStore) ATHubUsageRange(_ context.Context, user uuid.UUID, from, until time.Time, bucket int) (store.ATHubUsage, error) {
	s.queriedUser, s.from, s.until, s.bucket = user, from, until, bucket
	return store.ATHubUsage{From: &from, Until: &until, BucketSeconds: bucket}, nil
}

func TestATHubUsageRangeValidation(t *testing.T) {
	for _, days := range []string{"1", "7", "30"} {
		a, s := keyAPI(t)
		r := keyRequest("GET", "")
		r.URL.RawQuery = "days=" + days
		w := httptest.NewRecorder()
		a.athubUsage(w, r)
		wantBucket := 86400
		if days == "1" {
			wantBucket = 3600
		}
		if w.Code != 200 || s.queriedUser != s.user.ID || s.bucket != wantBucket || !s.until.After(s.from) {
			t.Fatalf("days=%s status=%d bucket=%d", days, w.Code, s.bucket)
		}
	}
	for _, query := range []string{"days=0", "days=31", "days=x", "days=1&days=7", "user_id=spoofed", "days=%zz"} {
		a, _ := keyAPI(t)
		r := keyRequest("GET", "")
		r.URL.RawQuery = query
		w := httptest.NewRecorder()
		a.athubUsage(w, r)
		if w.Code != 400 {
			t.Fatalf("query=%s status=%d", query, w.Code)
		}
	}
}
func (s *keyStore) ListATHubCalls(_ context.Context, user uuid.UUID, _, _ int) ([]store.ATHubCall, error) {
	s.queriedUser = user
	return []store.ATHubCall{}, nil
}

func keyAPI(t *testing.T) (*API, *keyStore) {
	t.Helper()
	s := &keyStore{user: store.User{ID: uuid.New(), IsActive: true}}
	return &API{store: s, cfg: config.Config{SessionCookieName: "session"}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}, s
}
func keyRequest(method, body string) *http.Request {
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "session", Value: "logged-in"})
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-AlbaTextus-Client", "account-center")
	return r
}

func TestATHubKeyCreationReturnsSecretOnlyOnce(t *testing.T) {
	a, s := keyAPI(t)
	w := httptest.NewRecorder()
	a.createATHubKey(w, keyRequest("POST", `{"name":"采集任务"}`))
	if w.Code != 201 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var created struct {
		APIKey string         `json:"api_key"`
		Key    store.ATHubKey `json:"key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.APIKey, "ath_") || len(created.APIKey) != 47 || s.key.TokenHash != security.HashToken(created.APIKey) || s.key.UserID != s.user.ID {
		t.Fatal("key was not generated, hashed or assigned to current user")
	}
	if created.Key.TokenHash != "" || strings.Contains(w.Body.String(), "token_hash") {
		t.Fatal("hash leaked in create response")
	}
	w = httptest.NewRecorder()
	a.listATHubKeys(w, keyRequest("GET", ""))
	if w.Code != 200 || strings.Contains(w.Body.String(), created.APIKey) || strings.Contains(w.Body.String(), s.key.TokenHash) {
		t.Fatal("key secret or hash leaked in list response")
	}
	if s.queriedUser != s.user.ID {
		t.Fatal("list was not scoped to current user")
	}
	w = httptest.NewRecorder()
	a.createATHubKey(w, keyRequest("POST", `{}`))
	if w.Code != 200 || strings.Contains(w.Body.String(), "api_key") || s.key.ID != created.Key.ID {
		t.Fatal("repeated provisioning changed key or exposed secret")
	}
	now := time.Now()
	s.key.RevokedAt = &now
	w = httptest.NewRecorder()
	a.createATHubKey(w, keyRequest("POST", `{}`))
	if w.Code != 200 || s.key.RevokedAt == nil {
		t.Fatal("provisioning reenabled disabled key")
	}
	w = httptest.NewRecorder()
	a.resetATHubKey(w, keyRequest("POST", `{}`))
	if w.Code != 201 || s.key.ID == created.Key.ID || s.key.RevokedAt != nil || s.key.ExpiresAt != nil {
		t.Fatal("reset did not replace disabled key with permanent key")
	}
}

func TestATHubKeyManagementRequiresSessionAndOrigin(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		cookie, header, crossSite bool
		status                    int
	}{
		{"no session", false, true, false, 401},
		{"missing request header", true, false, false, 403},
		{"cross site", true, true, true, 403},
		{"disabled user", true, true, false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := keyAPI(t)
			r := keyRequest("POST", `{"name":"key"}`)
			if !tc.cookie {
				r.Header.Del("Cookie")
				r.Header.Set("Authorization", "Bearer oauth-token")
			}
			if !tc.header {
				r.Header.Del("X-AlbaTextus-Client")
			}
			if tc.crossSite {
				r.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			if tc.name == "disabled user" {
				a.store.(*keyStore).user.IsActive = false
			}
			w := httptest.NewRecorder()
			a.createATHubKey(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d", w.Code, tc.status)
			}
		})
	}
}

func TestATHubKeyValidationAndOwnership(t *testing.T) {
	a, s := keyAPI(t)
	for _, body := range []string{`{"name":""}`, `{"name":"key","user_id":"spoofed"}`, `{"name":"key","expires_at":"2000-01-01T00:00:00Z"}`} {
		w := httptest.NewRecorder()
		a.createATHubKey(w, keyRequest("POST", body))
		if w.Code != 400 {
			t.Fatalf("body=%s status=%d", body, w.Code)
		}
	}
	s.key = store.ATHubKey{ID: uuid.New(), UserID: uuid.New()}
	r := keyRequest("DELETE", "")
	params := chi.NewRouteContext()
	params.URLParams.Add("id", s.key.ID.String())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, params))
	w := httptest.NewRecorder()
	a.revokeATHubKey(w, r)
	if w.Code != 404 || s.key.RevokedAt != nil {
		t.Fatal("another user's key was revoked")
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=2&limit=3", "?token=secret", "?limit=%zz"} {
		r := keyRequest("GET", "")
		r.URL.RawQuery = strings.TrimPrefix(query, "?")
		w := httptest.NewRecorder()
		a.athubCalls(w, r)
		if w.Code != 400 {
			t.Fatalf("query=%s status=%d", query, w.Code)
		}
	}
}
