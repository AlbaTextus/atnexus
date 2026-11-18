package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type reviewAvatarStore struct {
	*policyAPIStore
	review   store.ATHubReview
	reviewer store.User
	missing  bool
	lookups  int
	filter   string
	offset   int
}

func (s *reviewAvatarStore) ATHubReview(context.Context, uuid.UUID, uuid.UUID, bool) (store.ATHubReview, error) {
	return s.review, nil
}

func (s *reviewAvatarStore) UserByID(ctx context.Context, id uuid.UUID) (store.User, error) {
	if id == s.reviewer.ID {
		s.lookups++
		if s.missing {
			return store.User{}, store.ErrNotFound
		}
		return s.reviewer, nil
	}
	return s.keyStore.UserByID(ctx, id)
}

func (s *reviewAvatarStore) ListATHubReviews(_ context.Context, _ uuid.UUID, _ bool, status string, offset int) (store.ATHubReviewList, error) {
	s.filter, s.offset = status, offset
	second := s.review
	second.ID = uuid.New()
	return store.ATHubReviewList{Items: []store.ATHubReview{s.review, second}, Total: 2}, nil
}

func TestAllReviewsResolvesHistoricalContributorAndReviewerAvatars(t *testing.T) {
	a, keys := keyAPI(t)
	keys.user.Role = "ADMIN"
	keys.user.AvatarURL = "https://example.test/contributor.png"
	reviewerID := uuid.New()
	reviewer, _ := json.Marshal(map[string]string{"id": reviewerID.String(), "name": "审核人", "role": "ADMIN"})
	// Older contributions may expose a public ID instead of the account UUID.
	contributor := json.RawMessage(`{"id":"public-contributor","name":"提交者","avatar":""}`)
	s := &reviewAvatarStore{
		policyAPIStore: &policyAPIStore{keyStore: keys},
		review: store.ATHubReview{ID: uuid.New(), UserID: &keys.user.ID,
			Contributor: contributor, Reviewer: reviewer, Status: "approved"},
		reviewer: store.User{ID: reviewerID, AvatarURL: "https://example.test/reviewer.png", Email: "private@example.test"},
	}
	a.Store = s
	request := keyRequest("GET", "")
	request.URL.Path = "/api/v4/admin/athub/reviews"
	request.URL.RawQuery = "offset=20"
	response := httptest.NewRecorder()
	a.athubReviews(response, request)
	if response.Code != 200 || s.filter != "" || s.offset != 20 {
		t.Fatalf("all reviews: status=%d filter=%q offset=%d body=%s", response.Code, s.filter, s.offset, response.Body)
	}
	var result struct {
		Items []struct {
			Contributor struct{ ID, Name, Avatar string }
			Reviewer    struct{ Name, Avatar string }
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || s.lookups != 1 {
		t.Fatalf("items=%d reviewer lookups=%d", len(result.Items), s.lookups)
	}
	for _, item := range result.Items {
		if item.Contributor.ID != "public-contributor" || item.Contributor.Name != "提交者" ||
			item.Contributor.Avatar != keys.user.AvatarURL || item.Reviewer.Avatar != s.reviewer.AvatarURL {
			t.Fatalf("unexpected identities: %+v", item)
		}
	}
	if strings.Contains(response.Body.String(), s.reviewer.Email) {
		t.Fatal("private account information leaked")
	}
}

func TestHistoricalReviewResolvesSelectedAvatar(t *testing.T) {
	for _, missing := range []bool{false, true} {
		a, keys := keyAPI(t)
		keys.user.Role = "ADMIN"
		reviewerID := uuid.New()
		identity, _ := json.Marshal(map[string]string{"id": reviewerID.String(), "name": "审核人", "role": "ADMIN"})
		s := &reviewAvatarStore{
			policyAPIStore: &policyAPIStore{keyStore: keys},
			review:         store.ATHubReview{ID: uuid.New(), Status: "approved", Reviewer: identity},
			reviewer:       store.User{ID: reviewerID, AvatarURL: "https://example.test/selected-avatar.png", Email: "private@example.test"},
			missing:        missing,
		}
		a.Store = s
		request := keyRequest("GET", "")
		request.URL.Path = "/api/v4/admin/athub/reviews/" + s.review.ID.String()
		params := chi.NewRouteContext()
		params.URLParams.Add("id", s.review.ID.String())
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, params))
		response := httptest.NewRecorder()
		a.athubReviews(response, request)
		if response.Code != 200 {
			t.Fatalf("missing=%v status=%d body=%s", missing, response.Code, response.Body)
		}
		var result struct {
			Reviewer struct{ Name, Avatar string }
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Reviewer.Name != "审核人" || (!missing && result.Reviewer.Avatar != s.reviewer.AvatarURL) || (missing && result.Reviewer.Avatar != "") {
			t.Fatalf("unexpected reviewer: %+v", result.Reviewer)
		}
		if strings.Contains(response.Body.String(), s.reviewer.Email) {
			t.Fatal("private account information leaked into reviewer identity")
		}
	}
}
