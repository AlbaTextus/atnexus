package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const testCookie, testToken = "session", "session-token"

// codeStore records what the handlers ask of the account database.
type codeStore struct {
	store.Store
	user      store.User
	reserved  []store.EmailChallenge
	ready     []uuid.UUID
	deleted   []uuid.UUID
	completed []store.EmailCompletion
	reserve   error
}

func (s *codeStore) Ping(context.Context) error { return nil }
func (s *codeStore) SessionByHash(_ context.Context, hash string) (store.Session, error) {
	if s.user.ID == uuid.Nil || hash != security.HashToken(testToken) {
		return store.Session{}, store.ErrNotFound
	}
	return store.Session{UserID: s.user.ID, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (s *codeStore) UserByID(context.Context, uuid.UUID) (store.User, error) { return s.user, nil }
func (s *codeStore) ReserveEmailCode(_ context.Context, challenge store.EmailChallenge, _ []store.EmailRateLimit) error {
	if s.reserve != nil {
		return s.reserve
	}
	s.reserved = append(s.reserved, challenge)
	return nil
}
func (s *codeStore) ReadyEmailCode(_ context.Context, id uuid.UUID) error {
	s.ready = append(s.ready, id)
	return nil
}
func (s *codeStore) DeleteEmailCode(_ context.Context, id uuid.UUID) error {
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *codeStore) CompleteEmailVerification(_ context.Context, completion store.EmailCompletion) (store.User, error) {
	s.completed = append(s.completed, completion)
	user := s.user
	user.Email, user.EmailVerified = completion.Email, true
	return user, nil
}

type sentCode struct{ to, code, purpose string }

type recordingSender struct {
	sent []sentCode
	err  error
}

func (s *recordingSender) SendCode(_ context.Context, to, code, purpose string) error {
	s.sent = append(s.sent, sentCode{to, code, purpose})
	return s.err
}

// siteRules stands in for the account module.
type siteRules struct{ registrationClosed, robot, maintenance bool }

func (s siteRules) RequireRegistration(w http.ResponseWriter, _ *http.Request) bool {
	if s.registrationClosed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "registration_disabled"})
	}
	return !s.registrationClosed
}
func (s siteRules) VerifyHuman(w http.ResponseWriter, _ *http.Request) bool {
	if s.robot {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "turnstile_failed"})
	}
	return !s.robot
}
func (s siteRules) MaintenanceGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.maintenance {
			writeJSON(w, 433, map[string]string{"error": "maintenance"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (siteRules) UserView(user store.User) map[string]any {
	return map[string]any{"email": user.Email, "email_verified": user.EmailVerified}
}

func mailAPI(rules siteRules) (*API, *codeStore, *recordingSender) {
	st, sender := &codeStore{}, &recordingSender{}
	cfg := config.Config{SessionCookieName: testCookie, SMTP: config.SMTP{Host: "smtp.test", User: "sender@smtp.test", Pass: "secret"}}
	return &API{Base: &apibase.Base{Config: cfg, Store: st, Log: slog.Default()}, account: rules, mailer: sender}, st, sender
}

func post(a *API, path, body string, signedIn bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-AlbaTextus-Client", "account-center")
	if signedIn {
		r.AddCookie(&http.Cookie{Name: testCookie, Value: testToken})
	}
	w := httptest.NewRecorder()
	a.Router().ServeHTTP(w, r)
	return w
}

func TestRoutesStayUnderV3(t *testing.T) {
	a, _, _ := mailAPI(siteRules{})
	want := map[string]bool{
		"GET /api/v3/healthz":             true,
		"POST /api/v3/auth/email/code":    true,
		"POST /api/v3/account/email/code": true,
		"POST /api/v3/account/email":      true,
	}
	err := chi.Walk(a.Router().(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !want[method+" "+route] {
			t.Errorf("unexpected route: %s %s", method, route)
		}
		delete(want, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for route := range want {
		t.Errorf("route is missing: %s", route)
	}
}

func TestRegistrationCodeIsSentAndMadeReady(t *testing.T) {
	a, st, sender := mailAPI(siteRules{})
	w := post(a, "/api/v3/auth/email/code", `{"email":" Reader@Example.com "}`, false)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"challenge_id"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(sender.sent) != 1 || sender.sent[0].to != "reader@example.com" || sender.sent[0].purpose != "register" || len(sender.sent[0].code) != 6 {
		t.Fatalf("sent=%+v", sender.sent)
	}
	if len(st.reserved) != 1 || st.reserved[0].Purpose != "register" || st.reserved[0].Actor != "register" {
		t.Fatalf("reserved=%+v", st.reserved)
	}
	// Only the keyed digest is stored, and the code becomes usable after delivery.
	if st.reserved[0].CodeHash == "" || st.reserved[0].CodeHash == sender.sent[0].code {
		t.Fatal("the code was stored without hashing")
	}
	if len(st.ready) != 1 || st.ready[0] != st.reserved[0].ID || len(st.deleted) != 0 {
		t.Fatalf("ready=%v deleted=%v", st.ready, st.deleted)
	}
}

func TestRegistrationCodeFollowsSiteRules(t *testing.T) {
	for name, tc := range map[string]struct {
		rules  siteRules
		status int
		code   string
	}{
		"registration closed": {siteRules{registrationClosed: true}, http.StatusForbidden, "registration_disabled"},
		"human check failed":  {siteRules{robot: true}, http.StatusForbidden, "turnstile_failed"},
		"maintenance":         {siteRules{maintenance: true}, 433, "maintenance"},
	} {
		t.Run(name, func(t *testing.T) {
			a, st, sender := mailAPI(tc.rules)
			w := post(a, "/api/v3/auth/email/code", `{"email":"reader@example.com"}`, false)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if len(sender.sent) != 0 || len(st.reserved) != 0 {
				t.Fatal("a refused request still reserved or sent a code")
			}
		})
	}
}

func TestHealthIgnoresMaintenance(t *testing.T) {
	a, _, _ := mailAPI(siteRules{maintenance: true})
	w := httptest.NewRecorder()
	a.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v3/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCodeIsRefusedBeforeSending(t *testing.T) {
	t.Run("SMTP not configured", func(t *testing.T) {
		a, _, sender := mailAPI(siteRules{})
		a.Config.SMTP = config.SMTP{}
		w := post(a, "/api/v3/auth/email/code", `{"email":"reader@example.com"}`, false)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "email_unavailable") || len(sender.sent) != 0 {
			t.Fatalf("status=%d body=%s sent=%d", w.Code, w.Body.String(), len(sender.sent))
		}
	})
	t.Run("invalid address", func(t *testing.T) {
		a, _, sender := mailAPI(siteRules{})
		for _, address := range []string{"", "reader", "Reader <reader@example.com>", "reader@accounts.albatextus.invalid"} {
			w := post(a, "/api/v3/auth/email/code", `{"email":"`+address+`"}`, false)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_email") {
				t.Errorf("%q: status=%d body=%s", address, w.Code, w.Body.String())
			}
		}
		if len(sender.sent) != 0 {
			t.Fatal("an invalid address was sent a code")
		}
	})
	t.Run("rate limited", func(t *testing.T) {
		a, st, sender := mailAPI(siteRules{})
		st.reserve = store.ErrEmailRateLimit
		w := post(a, "/api/v3/auth/email/code", `{"email":"reader@example.com"}`, false)
		if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" || len(sender.sent) != 0 {
			t.Fatalf("status=%d retry=%q sent=%d", w.Code, w.Header().Get("Retry-After"), len(sender.sent))
		}
	})
}

func TestFailedDeliveryLeavesNoUsableCode(t *testing.T) {
	a, st, sender := mailAPI(siteRules{})
	sender.err = errors.New("SMTP delivery failed")
	w := post(a, "/api/v3/auth/email/code", `{"email":"reader@example.com"}`, false)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "email_unavailable") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(st.ready) != 0 || len(st.deleted) != 1 || st.deleted[0] != st.reserved[0].ID {
		t.Fatalf("ready=%v deleted=%v", st.ready, st.deleted)
	}
}

func TestAccountRoutesRequireSession(t *testing.T) {
	for _, path := range []string{"/api/v3/account/email/code", "/api/v3/account/email"} {
		a, _, sender := mailAPI(siteRules{})
		w := post(a, path, `{"email":"reader@example.com"}`, false)
		if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "login_required") || len(sender.sent) != 0 {
			t.Errorf("%s: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestAccountCodeIsBoundToUserAndSession(t *testing.T) {
	a, st, sender := mailAPI(siteRules{})
	st.user = store.User{ID: uuid.New(), Email: "old@example.com", IsActive: true}
	w := post(a, "/api/v3/account/email/code", `{"email":"new@example.com"}`, true)
	if w.Code != http.StatusOK || len(sender.sent) != 1 || sender.sent[0].purpose != "account" {
		t.Fatalf("status=%d body=%s sent=%+v", w.Code, w.Body.String(), sender.sent)
	}
	challenge := st.reserved[0]
	if challenge.Actor != st.user.ID.String()+":"+security.HashToken(testToken) || challenge.PreviousEmail != "old@example.com" || challenge.Email != "new@example.com" {
		t.Fatalf("challenge=%+v", challenge)
	}
}

func TestConfirmAccountEmail(t *testing.T) {
	a, st, _ := mailAPI(siteRules{})
	st.user = store.User{ID: uuid.New(), Email: "old@example.com", IsActive: true}
	w := post(a, "/api/v3/account/email", `{"email":"new@example.com","code":"012345","challenge_id":"not-a-uuid"}`, true)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_email_code") || len(st.completed) != 0 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	id := uuid.New()
	w = post(a, "/api/v3/account/email", `{"email":"New@Example.com","code":"012345","challenge_id":"`+id.String()+`"}`, true)
	// The reply is the account module's view of the updated user.
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"email":"new@example.com"`) || !strings.Contains(w.Body.String(), `"email_verified":true`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	done := st.completed[0]
	if done.ID != id || done.Purpose != "account" || done.UserID != st.user.ID || done.SessionHash != security.HashToken(testToken) || done.CodeHash != security.EmailCodeHash(a.Config.ClientSecretEncryptionKey, id.String(), "012345") {
		t.Fatalf("completion=%+v", done)
	}
}
