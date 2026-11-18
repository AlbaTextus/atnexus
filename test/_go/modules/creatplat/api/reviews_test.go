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
	for _, scenario := range []string{"no session", "missing origin", "cross site", "missing rejection reason", "valid approval", "valid rejection"} {
		a, keys := keyAPI(t)
		keys.user.Role = "ADMIN"
		s := &policyAPIStore{keyStore: keys}
		a.Store = s
		var forgotten []string
		a.SetATHubReceiptForgetter(func(_ context.Context, id string) error {
			forgotten = append(forgotten, id)
			return nil
		})
		body := `{"decision":"approve"}`
		switch scenario {
		case "missing rejection reason":
			body = `{"decision":"reject","note":""}`
		case "valid rejection":
			body = `{"decision":"reject","note":"duplicate"}`
		}
		reviewID := uuid.NewString()
		request := keyRequest("POST", body)
		request.URL.Path = "/api/v4/admin/athub/reviews/" + reviewID
		params := chi.NewRouteContext()
		params.URLParams.Add("id", reviewID)
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
		case "valid approval", "valid rejection":
			want = 200
		}
		w := httptest.NewRecorder()
		a.athubReviews(w, route)
		if w.Code != want || s.decided != (want == 200) {
			t.Fatalf("scenario=%s status=%d body=%s", scenario, w.Code, w.Body)
		}
		// Only a committed approval has a content receipt to release.
		if scenario == "valid approval" {
			if len(forgotten) != 1 || forgotten[0] != reviewID {
				t.Fatalf("receipts released after approval = %v, want [%s]", forgotten, reviewID)
			}
		} else if len(forgotten) != 0 {
			t.Fatalf("scenario=%s released receipts %v", scenario, forgotten)
		}
		if want == 200 && s.finishedStatus != 200 {
			t.Fatal("review audit not completed")
		}
	}
}
