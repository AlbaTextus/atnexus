package athubaccess

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/google/uuid"
)

type sessionVerifier struct {
	identity auth.Identity
	err      error
}

func (s sessionVerifier) VerifySession(*http.Request) (auth.Identity, error) {
	return s.identity, s.err
}

func TestOAuthSessionAdmission(t *testing.T) {
	for _, scenario := range []struct {
		name, method string
		err          error
		csrf         bool
		want         int
	}{
		{"read", "GET", nil, false, 204},
		{"write with csrf", "PUT", nil, true, 204},
		{"write without csrf", "PUT", nil, false, 403},
		{"revoked session", "GET", auth.ErrUnauthenticated, false, 401},
		{"session authority unavailable", "GET", errors.New("database offline"), false, 503},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			st := &accessStore{}
			a := accessFor(st)
			identity := auth.Identity{UserID: uuid.New(), SessionID: uuid.New()}
			a.EnableSessions(sessionVerifier{identity, scenario.err})
			dispatched := false
			handler := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { dispatched = true; w.WriteHeader(204) }))
			r := httptest.NewRequest(scenario.method, "/api/v2/content", nil)
			if scenario.csrf {
				r.Header.Set("X-AlbaTextus-Client", "account-center")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != scenario.want || dispatched != (scenario.want == 204) {
				t.Fatalf("status=%d dispatched=%v body=%s", w.Code, dispatched, w.Body)
			}
			if scenario.want == 204 && (st.call.KeyID != nil || st.call.UserID == nil || *st.call.UserID != identity.UserID || st.call.SessionID == nil || *st.call.SessionID != identity.SessionID || !st.finished) {
				t.Fatalf("session audit attribution=%+v", st.call)
			}
		})
	}
}

func TestCreatorHistoryUsesOAuthSession(t *testing.T) {
	st, creator := &accessStore{}, &historyReads{}
	access := New(st, nil)
	access.EnableCreatorStorage(creator)
	identity := auth.Identity{UserID: uuid.New(), SessionID: uuid.New()}
	access.EnableSessions(sessionVerifier{identity: identity})
	access.Wrap(http.NotFoundHandler())
	w := httptest.NewRecorder()
	access.ServeHistory(w, httptest.NewRequest("GET", "/api/v4/history?novelId=7&limit=10", nil))
	if w.Code != 200 || creator.novelID != 7 || st.call.Path != "/api/v3/history" || !st.finished || st.call.KeyID != nil || st.call.SessionID == nil || *st.call.SessionID != identity.SessionID {
		t.Fatalf("status=%d audit=%+v body=%s", w.Code, st.call, w.Body)
	}
}
