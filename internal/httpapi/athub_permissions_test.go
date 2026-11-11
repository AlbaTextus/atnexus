package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

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

func TestATHubPolicyAdministrationAccess(t *testing.T) {
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		for _, method := range []string{"GET", "PUT"} {
			a, keys := keyAPI(t)
			keys.user.Role = role
			s := &policyAPIStore{keyStore: keys}
			a.store = s
			request := keyRequest(method, `{"role":"USER","rules":[]}`)
			request.URL.Path = "/api/v1/admin/athub/permissions"
			w := httptest.NewRecorder()
			a.athubPermissions(w, request)
			want := 403
			if role == "SUPER_ADMIN" || (role == "ADMIN" && method == "GET") {
				want = 200
			}
			if w.Code != want || (s.updated && (role != "SUPER_ADMIN" || method != "PUT")) {
				t.Fatalf("role=%s method=%s status=%d updated=%v", role, method, w.Code, s.updated)
			}
		}
	}
}
func TestATHubReviewHTTPAccessAndValidation(t *testing.T) {
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		a, keys := keyAPI(t)
		keys.user.Role = role
		s := &policyAPIStore{keyStore: keys}
		a.store = s
		request := keyRequest("GET", "")
		request.URL.Path = "/api/v1/admin/athub/reviews"
		w := httptest.NewRecorder()
		a.athubReviews(w, request)
		want := 403
		if role == "ADMIN" || role == "SUPER_ADMIN" {
			want = 200
		}
		if w.Code != want || s.listed != (want == 200) {
			t.Fatalf("role=%s status=%d listed=%v", role, w.Code, s.listed)
		}
	}
	for _, scenario := range []string{"no session", "missing origin", "cross site", "missing rejection reason", "valid approval"} {
		a, keys := keyAPI(t)
		keys.user.Role = "ADMIN"
		s := &policyAPIStore{keyStore: keys}
		a.store = s
		body := `{"decision":"approve"}`
		if scenario == "missing rejection reason" {
			body = `{"decision":"reject","note":""}`
		}
		request := keyRequest("POST", body)
		request.URL.Path = "/api/v1/admin/athub/reviews/" + uuid.NewString()
		params := chi.NewRouteContext()
		params.URLParams.Add("id", uuid.NewString())
		route := request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, params))
		want := 403
		switch scenario {
		case "no session":
			route.Header.Del("Cookie")
			want = 401
		case "missing origin":
			route.Header.Del("X-AlbaTextus-Client")
		case "cross site":
			route.Header.Set("Sec-Fetch-Site", "cross-site")
		case "missing rejection reason":
			want = 400
		case "valid approval":
			want = 200
		}
		w := httptest.NewRecorder()
		a.athubReviews(w, route)
		if w.Code != want || s.decided != (want == 200) {
			t.Fatalf("scenario=%s status=%d body=%s", scenario, w.Code, w.Body)
		}
		if want == 200 && s.finishedStatus != 200 {
			t.Fatal("review audit not completed")
		}
	}
}
