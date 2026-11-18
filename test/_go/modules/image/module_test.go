package image

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type sessionStub struct {
	identity auth.Identity
	err      error
}

func (s sessionStub) VerifySession(*http.Request) (auth.Identity, error) { return s.identity, s.err }

func sessionOf(role string) sessionStub {
	return sessionStub{identity: auth.Identity{UserID: uuid.New(), Role: role}}
}

var noSession = sessionStub{err: auth.ErrUnauthenticated}

// keyStub lists the permissions granted to each known key.
type keyStub map[string][]string

func (k keyStub) AuthorizeImageKey(_ context.Context, raw, permission string) (string, error) {
	granted, known := k[raw]
	if !known {
		return "", auth.ErrUnauthenticated
	}
	if !slices.Contains(granted, permission) {
		return "", auth.ErrForbidden
	}
	return raw, nil
}

func moduleWith(sessions sessionStub, keys keyStub) *Module {
	return &Module{deps: Dependencies{Sessions: sessions, Keys: keys}}
}

// admit sends one request through authorize. It returns the status and error
// code of the reply, and the actor the handler saw, or nil when it was refused.
func admit(m *Module, permission string, r *http.Request) (int, string, *actor) {
	var seen *actor
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		who := r.Context().Value(actorKey{}).(actor)
		seen = &who
	})
	w := httptest.NewRecorder()
	m.authorize(permission, next).ServeHTTP(w, r)
	return w.Code, errorCode(w), seen
}

func errorCode(w *httptest.ResponseRecorder) string {
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error
}

// fromAccountCenter builds a request with the header the account center adds
// to everything it sends.
func fromAccountCenter(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set("X-AlbaTextus-Client", "account-center")
	return r
}

// Accounts below administrator have their images stored by OAuth and creatplat
// through Save. Over HTTP a session is an administrator's or it is refused,
// whichever permission the route asks for.
func TestSessionsAreAdmittedOnlyForAdministrators(t *testing.T) {
	for _, tc := range []struct {
		role     string
		admitted bool
	}{{"USER", false}, {"LEVEL_2", false}, {"", false}, {"ADMIN", true}, {"SUPER_ADMIN", true}} {
		for _, permission := range []string{"upload", "list", "delete", "manage"} {
			sessions := sessionOf(tc.role)
			status, code, who := admit(moduleWith(sessions, nil), permission, fromAccountCenter("POST", "/api/v2/upload"))
			if !tc.admitted {
				if status != 403 || code != "permission_denied" || who != nil {
					t.Errorf("role %q asking for %s: status=%d error=%q reached handler=%v, want 403 permission_denied", tc.role, permission, status, code, who != nil)
				}
				continue
			}
			if who == nil || !who.Admin || who.ID != "user:"+sessions.identity.UserID.String() {
				t.Errorf("role %q asking for %s: status=%d error=%q actor=%+v, want the administrator admitted", tc.role, permission, status, code, who)
			}
		}
	}
}

func TestAdministratorSessionsMustComeFromTheAccountCenter(t *testing.T) {
	m := moduleWith(sessionOf("ADMIN"), nil)
	crossSite := fromAccountCenter("POST", "/api/v2/upload")
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	for name, r := range map[string]*http.Request{"no client header": httptest.NewRequest("POST", "/api/v2/upload", nil), "cross-site": crossSite} {
		if status, code, who := admit(m, "upload", r); status != 403 || code != "invalid_origin" || who != nil {
			t.Errorf("%s: status=%d error=%q reached handler=%v, want 403 invalid_origin", name, status, code, who != nil)
		}
	}
	if _, _, who := admit(m, "list", httptest.NewRequest("GET", "/api/v2/manage/list", nil)); who == nil {
		t.Error("an administrator's read was refused for lacking the client header")
	}
}

// Keys are issued by administrators with explicit permissions. A key is judged
// alone: it needs no browser header and never borrows a session's standing.
func TestKeysAreAdmittedByTheirOwnPermissions(t *testing.T) {
	keys := keyStub{"uploader": {"upload"}, "operator": {"upload", "list", "delete", "manage"}}
	for _, tc := range []struct {
		name, key, permission string
		sessions              sessionStub
		status                int
		code                  string
		admin                 bool
	}{
		{"upload key uploads", "uploader", "upload", noSession, 200, "", false},
		{"upload key beside an ordinary session", "uploader", "upload", sessionOf("USER"), 200, "", false},
		{"upload key cannot delete", "uploader", "delete", noSession, 403, "permission_denied", false},
		{"manage key manages", "operator", "manage", noSession, 200, "", true},
		{"unknown key beside an administrator session", "stolen", "upload", sessionOf("ADMIN"), 401, "invalid_key", false},
	} {
		r := httptest.NewRequest("POST", "/api/v2/upload", nil)
		r.Header.Set("Authorization", "Bearer "+tc.key)
		status, code, who := admit(moduleWith(tc.sessions, keys), tc.permission, r)
		if status != tc.status || code != tc.code {
			t.Errorf("%s: status=%d error=%q, want %d %q", tc.name, status, code, tc.status, tc.code)
			continue
		}
		if admitted := tc.status == 200; admitted != (who != nil) || admitted && (who.ID != "key:"+tc.key || who.Admin != tc.admin) {
			t.Errorf("%s: actor=%+v, want admitted=%v admin=%v", tc.name, who, admitted, tc.admin)
		}
	}
}

func TestRequestsWithoutAUsableSessionAreRefused(t *testing.T) {
	for _, tc := range []struct {
		sessions sessionStub
		status   int
		code     string
	}{
		{noSession, 401, "login_required"},
		{sessionStub{err: errors.New("account database unavailable")}, 503, "authentication_unavailable"},
	} {
		if status, code, who := admit(moduleWith(tc.sessions, nil), "upload", fromAccountCenter("POST", "/api/v2/upload")); status != tc.status || code != tc.code || who != nil {
			t.Errorf("status=%d error=%q reached handler=%v, want %d %s", status, code, who != nil, tc.status, tc.code)
		}
	}
}

// Every route but the health probe goes through authorize, so none is left
// open to an ordinary account or to a caller without credentials.
func TestOnlyTheHealthProbeIsOpen(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sessions sessionStub
		status   int
		code     string
	}{
		{"ordinary account", sessionOf("USER"), 403, "permission_denied"},
		{"no credentials", noSession, 401, "login_required"},
	} {
		router := moduleWith(tc.sessions, nil).Router()
		checked := map[string]bool{}
		err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if route == "/api/v2/healthz" {
				return nil
			}
			checked[route] = true
			w := httptest.NewRecorder()
			reached := func() (reached bool) {
				// A handler that ran would fail here: this module has no database.
				defer func() { reached = recover() != nil }()
				router.ServeHTTP(w, fromAccountCenter(method, strings.Replace(route, "*", "folder/file.png", 1)))
				return false
			}()
			if reached || w.Code != tc.status || errorCode(w) != tc.code {
				t.Errorf("%s: %s %s: reached handler=%v status=%d error=%q, want %d %s", tc.name, method, route, reached, w.Code, errorCode(w), tc.status, tc.code)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range []string{"/api/v2/upload", "/api/v2/categories/user/avatars/upload", "/api/v2/categories/book/covers/upload", "/api/v2/channels"} {
			if !checked[route] {
				t.Errorf("%s: %s was not checked", tc.name, route)
			}
		}
	}
}
