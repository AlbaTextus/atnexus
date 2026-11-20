package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/store"
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

func (s *keyStore) ListATHubCalls(_ context.Context, user uuid.UUID, _, _ int) ([]store.ATHubCall, error) {
	s.queriedUser = user
	return []store.ATHubCall{}, nil
}

func keyAPI(t *testing.T) (*API, *keyStore) {
	t.Helper()
	s := &keyStore{user: store.User{ID: uuid.New(), IsActive: true}}
	return &API{Base: &apibase.Base{Store: s, Config: config.Config{SessionCookieName: "session"}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}}, s
}
func keyRequest(method, body string) *http.Request {
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "session", Value: "logged-in"})
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-AlbaTextus-Client", "account-center")
	return r
}

type policyAPIStore struct {
	*keyStore
	store.ATHubReviewStore
	updated, listed, decided bool
	finishedStatus           int
}

func (s *policyAPIStore) ATHubPolicies(context.Context) ([]store.ATHubRolePolicy, error) {
	return []store.ATHubRolePolicy{{Role: "USER", Rules: []store.ATHubAccessRule{}}}, nil
}
func (s *policyAPIStore) ATHubPolicy(_ context.Context, role string) (store.ATHubRolePolicy, error) {
	return store.ATHubRolePolicy{Role: role, Rules: []store.ATHubAccessRule{}}, nil
}
func (s *policyAPIStore) SetATHubPolicy(_ context.Context, _ uuid.UUID, policy store.ATHubRolePolicy) (store.ATHubRolePolicy, error) {
	s.updated = true
	return policy, nil
}
func (s *policyAPIStore) ListATHubReviews(context.Context, uuid.UUID, bool, string, int) (store.ATHubReviewList, error) {
	s.listed = true
	return store.ATHubReviewList{Items: []store.ATHubReview{}}, nil
}
func (s *policyAPIStore) DecideATHubReview(_ context.Context, _ uuid.UUID, id uuid.UUID, _ string, _ string, _ store.ATHubReviewExecutor) (store.ATHubReview, error) {
	s.decided = true
	return store.ATHubReview{ID: id, Status: "approved"}, nil
}
func (s *policyAPIStore) BeginATHubCall(context.Context, store.ATHubCall) error { return nil }
func (s *policyAPIStore) FinishATHubCall(_ context.Context, _ uuid.UUID, status int, _ int64, _ int64) error {
	s.finishedStatus = status
	return nil
}
