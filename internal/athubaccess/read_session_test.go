package athubaccess

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

func TestReadSessionCredentials(t *testing.T) {
	secret := "test-session-signing-secret"
	userID, sessionID := uuid.New(), uuid.New()
	credential, expires, err := IssueReadSession(secret, userID, sessionID, time.Now().Add(time.Minute))
	if err != nil || expires.After(time.Now().Add(time.Minute)) {
		t.Fatalf("unexpected lifetime: %v", err)
	}
	identity, ok := parseReadSession(secret, credential)
	if !ok || identity.UserID != userID || identity.SessionID != sessionID {
		t.Fatal("signed session did not round trip")
	}
	expired, _ := json.Marshal(ReadSession{userID, sessionID, time.Now().Add(-time.Minute).Unix()})
	payload := base64.RawURLEncoding.EncodeToString(expired)
	expiredCredential := "ats_" + payload + "." + base64.RawURLEncoding.EncodeToString(readSignature(secret, payload))
	for _, invalid := range []string{credential + "x", strings.Replace(credential, "ats_", "ath_", 1), expiredCredential, ""} {
		if _, ok := parseReadSession(secret, invalid); ok {
			t.Fatal("invalid session credential accepted")
		}
	}
	if _, ok := parseReadSession("different-secret", credential); ok {
		t.Fatal("wrong signing key accepted")
	}
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			st := &accessStore{}
			access := accessFor(t, st)
			access.EnableReadSessions(secret)
			dispatched := false
			handler := access.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched = true
				w.WriteHeader(204)
			}))
			request := httptest.NewRequest(method, "/api/v2/novelinfo", nil)
			request.Header.Set("Authorization", "Bearer "+credential)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if dispatched != (method == "GET") {
				t.Fatal("read session method scope bypassed")
			}
			if method == "GET" && (st.call.UserID == nil || *st.call.UserID != userID || st.call.SessionID == nil || *st.call.SessionID != sessionID || st.call.KeyID != nil) {
				t.Fatal("read audit must belong to session user, without personal key")
			}
			if !st.finished {
				t.Fatal("session call was not accounted")
			}
		})
	}
	t.Run("revoked before dispatch", func(t *testing.T) {
		st := &accessStore{beginErr: store.ErrATHubAccessDenied}
		access := accessFor(t, st)
		access.EnableReadSessions(secret)
		handler := access.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("denied session dispatched") }))
		request := httptest.NewRequest("GET", "/api/v2/content", nil)
		request.Header.Set("Authorization", "Bearer "+credential)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 401 || !st.finished {
			t.Fatal("revoked session was not rejected and accounted")
		}
	})
}
