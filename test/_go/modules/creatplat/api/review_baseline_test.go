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

type baselineReviewStore struct {
	*reviewAvatarStore
}

func (s *baselineReviewStore) DecideATHubReview(context.Context, uuid.UUID, uuid.UUID, string, string, store.ATHubReviewExecutor) (store.ATHubReview, error) {
	return s.review, nil
}

// The baseline is the host's record of what a submission alters, not part of
// the submission. Whatever a store hands back, no review endpoint serves it.
func TestReviewEndpointsNeverServeTheBaseline(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path, body string
		single                   bool
	}{
		{"all reviews", "GET", "/api/v4/admin/athub/reviews", "", false},
		{"own submissions", "GET", "/api/v4/account/athub/submissions", "", false},
		{"review detail", "GET", "/api/v4/admin/athub/reviews/", "", true},
		{"own submission detail", "GET", "/api/v4/account/athub/submissions/", "", true},
		{"decision", "POST", "/api/v4/admin/athub/reviews/", `{"decision":"reject","note":"重复"}`, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, keys := keyAPI(t)
			keys.user.Role = "ADMIN"
			author := uuid.New()
			review := store.ATHubReview{
				ID: uuid.New(), UserID: &author, Contributor: json.RawMessage(`{"name":"投稿者"}`),
				Operation: "edit", Method: "PUT", Path: "/api/v2/novelinfo", Query: "id=17",
				Payload: json.RawMessage(`{"title":"新标题"}`), Status: "pending",
				Baseline: json.RawMessage(`{"v":1,"novels":{"17":[{"before":{"title":"only-in-the-baseline"}}]}}`),
			}
			a.Store = &baselineReviewStore{&reviewAvatarStore{policyAPIStore: &policyAPIStore{keyStore: keys}, review: review}}
			request := keyRequest(scenario.method, scenario.body)
			request.URL.Path = scenario.path
			if scenario.single {
				request.URL.Path += review.ID.String()
				params := chi.NewRouteContext()
				params.URLParams.Add("id", review.ID.String())
				request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, params))
			}
			response := httptest.NewRecorder()
			a.athubReviews(response, request)
			body := response.Body.String()
			if response.Code != 200 || !strings.Contains(body, review.ID.String()) || strings.Contains(strings.ToLower(body), "baseline") {
				t.Fatalf("status=%d body=%s", response.Code, body)
			}
		})
	}
}
