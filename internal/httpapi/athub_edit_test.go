package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestNovelEditRequiresAccountAndCSRF(t *testing.T) {
	for _, scenario := range []struct {
		name string
		id   string
		want int
	}{
		{"valid session", "17", 202},
		{"no cookie", "17", 401},
		{"missing client header", "17", 403},
		{"cross site", "17", 403},
		{"revoked session", "17", 401},
		{"expired session", "17", 401},
		{"wrong session owner", "17", 401},
		{"service unavailable", "17", 503},
		{"invalid id", "abc", 400},
		{"zero id", "0", 400},
		{"negative id", "-1", 400},
		{"overflow id", "2147483648", 400},
		{"unexpected query", "17", 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, keys := keyAPI(t)
			st := &submitAPIStore{keyStore: keys, session: store.Session{
				ID: uuid.New(), UserID: keys.user.ID, ExpiresAt: time.Now().Add(time.Hour),
			}}
			a.store = st
			called := false
			a.SetATHubNovelEditor(func(w http.ResponseWriter, r *http.Request, user, session uuid.UUID, id int32) {
				called = true
				if user != keys.user.ID || session != st.session.ID || id != 17 {
					t.Fatal("edit was not bound to the account session and selected novel")
				}
				w.WriteHeader(202)
			})
			request := keyRequest("PUT", `{"title":"修改后的小说"}`)
			request.URL.Path = "/api/v1/account/athub/novels/" + scenario.id
			route := chi.NewRouteContext()
			route.URLParams.Add("id", scenario.id)
			request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
			switch scenario.name {
			case "no cookie":
				request.Header.Del("Cookie")
			case "missing client header":
				request.Header.Del("X-AlbaTextus-Client")
			case "cross site":
				request.Header.Set("Sec-Fetch-Site", "cross-site")
			case "revoked session":
				now := time.Now()
				st.session.RevokedAt = &now
			case "expired session":
				st.session.ExpiresAt = time.Now().Add(-time.Minute)
			case "wrong session owner":
				st.session.UserID = uuid.New()
			case "service unavailable":
				a.SetATHubNovelEditor(nil)
			case "unexpected query":
				request.URL.RawQuery = "id=99"
			}
			response := httptest.NewRecorder()
			a.athubEditNovel(response, request)
			if response.Code != scenario.want || called != (scenario.want == 202) {
				t.Fatalf("status=%d want=%d body=%s called=%v", response.Code, scenario.want, response.Body, called)
			}
		})
	}
}
