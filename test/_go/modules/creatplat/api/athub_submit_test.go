package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

type submitAPIStore struct {
	*keyStore
	session store.Session
}

func (s *submitAPIStore) SessionByHash(context.Context, string) (store.Session, error) {
	return s.session, nil
}

func TestSubmissionRequiresAccountAndCSRF(t *testing.T) {
	for _, scenario := range []struct {
		name string
		want int
	}{
		{"valid session", 202},
		{"no cookie", 401},
		{"missing client header", 403},
		{"cross site", 403},
		{"revoked session", 401},
		{"expired session", 401},
		{"submission service unavailable", 503},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, keys := keyAPI(t)
			st := &submitAPIStore{keyStore: keys, session: store.Session{
				ID: uuid.New(), UserID: keys.user.ID, ExpiresAt: time.Now().Add(time.Hour),
			}}
			a.Store = st
			called := false
			a.SetATHubSubmitter(func(w http.ResponseWriter, r *http.Request, user, session uuid.UUID) {
				called = true
				if user != keys.user.ID || session != st.session.ID {
					t.Fatal("submission session was not bound to the account")
				}
				w.WriteHeader(202)
			})
			request := keyRequest("POST", `{"novel":{"title":"测试小说"}}`)
			request.URL.Path = "/api/v4/account/athub/submissions"
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
			case "submission service unavailable":
				a.SetATHubSubmitter(nil)
			}
			response := httptest.NewRecorder()
			a.athubSubmit(response, request)
			if response.Code != scenario.want || called != (scenario.want == 202) {
				t.Fatalf("status=%d want=%d body=%s called=%v", response.Code, scenario.want, response.Body, called)
			}
		})
	}
}
