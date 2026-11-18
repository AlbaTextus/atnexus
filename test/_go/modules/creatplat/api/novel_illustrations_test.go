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

func TestIllustrationDeleteRequiresAccountSessionAndCSRF(t *testing.T) {
	for _, scenario := range []struct {
		name, novel, image string
		want               int
	}{
		{"valid", "17", "23", 204},
		{"missing cookie", "17", "23", 401},
		{"missing client header", "17", "23", 403},
		{"cross site", "17", "23", 403},
		{"revoked session", "17", "23", 401},
		{"wrong owner", "17", "23", 401},
		{"unavailable", "17", "23", 503},
		{"invalid novel", "0", "23", 400},
		{"invalid image", "17", "-1", 400},
		{"overflow image", "17", "2147483648", 400},
		{"unexpected query", "17", "23", 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, keys := keyAPI(t)
			st := &submitAPIStore{keyStore: keys, session: store.Session{
				ID: uuid.New(), UserID: keys.user.ID, ExpiresAt: time.Now().Add(time.Hour),
			}}
			a.Store = st
			called := false
			a.SetATHubIllustrationDeleter(func(w http.ResponseWriter, r *http.Request, user, session uuid.UUID, novelID, imageID int32) {
				called = true
				if user != keys.user.ID || session != st.session.ID || novelID != 17 || imageID != 23 {
					t.Fatal("deletion lost authenticated ownership or target")
				}
				w.WriteHeader(204)
			})
			request := keyRequest("DELETE", "")
			route := chi.NewRouteContext()
			route.URLParams.Add("id", scenario.novel)
			route.URLParams.Add("illustrationId", scenario.image)
			request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
			switch scenario.name {
			case "missing cookie":
				request.Header.Del("Cookie")
			case "missing client header":
				request.Header.Del("X-AlbaTextus-Client")
			case "cross site":
				request.Header.Set("Sec-Fetch-Site", "cross-site")
			case "revoked session":
				now := time.Now()
				st.session.RevokedAt = &now
			case "wrong owner":
				st.session.UserID = uuid.New()
			case "unavailable":
				a.SetATHubIllustrationDeleter(nil)
			case "unexpected query":
				request.URL.RawQuery = "id=99"
			}
			response := httptest.NewRecorder()
			a.deleteNovelIllustration(response, request)
			if response.Code != scenario.want || called != (scenario.want == 204) {
				t.Fatalf("status=%d body=%s dispatched=%v", response.Code, response.Body, called)
			}
		})
	}
}
