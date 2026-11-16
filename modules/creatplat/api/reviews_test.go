package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestATHubReviewHTTPAccessAndValidation(t *testing.T) {
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		a, keys := keyAPI(t)
		keys.user.Role = role
		s := &policyAPIStore{keyStore: keys}
		a.Store = s
		request := keyRequest("GET", "")
		request.URL.Path = "/api/v4/admin/athub/reviews"
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
		a.Store = s
		body := `{"decision":"approve"}`
		if scenario == "missing rejection reason" {
			body = `{"decision":"reject","note":""}`
		}
		request := keyRequest("POST", body)
		request.URL.Path = "/api/v4/admin/athub/reviews/" + uuid.NewString()
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
