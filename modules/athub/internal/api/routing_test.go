package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemovedAndLegacyRoutes(t *testing.T) {
	paths := []string{
		"/api/platforms", "/api/authors", "/api/novelinfo", "/api/volumes", "/api/toc", "/api/content", "/api/upload",
		"/api/v1/content", "/api/v2/athub/content",
		"/platforms", "/authors", "/novelinfo", "/volumes", "/toc", "/content", "/upload",
		"/stats", "/api/stats", "/account/ensure", "/account/keys", "/account/reveal", "/account/usage",
		"/api/account/ensure", "/api/account/keys", "/api/account/reveal", "/api/account/usage",
		"/api/auth/login", "/api/auth/callback", "/api/auth/logout", "/api/auth/session",
		"/api/v2/platforms/", "/api/api/v2/platforms", "/api", "/api/",
	}
	for _, path := range paths {
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			t.Run(method+path, func(t *testing.T) {
				// A nil store makes accidental dispatch to a storage handler fail.
				handler := testHandler(t, nil)
				w := request(handler, method, path, "{}", "Bearer "+testToken)
				if w.Code != http.StatusNotFound {
					t.Fatalf("got %d, want 404: %s", w.Code, w.Body.String())
				}
				w = request(handler, method, path, "{}", "")
				if w.Code != http.StatusUnauthorized {
					t.Fatalf("unauthenticated status %d, want 401", w.Code)
				}
			})
		}
	}
}

func TestOnlyServiceTokenAuthenticates(t *testing.T) {
	for _, path := range []string{"/api/v2/platforms", "/api/v2/authors", "/api/v2/novelinfo", "/api/v2/volumes", "/api/v2/toc", "/api/v2/content", "/api/v2/upload"} {
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			for _, auth := range []string{"", "Bearer ath_retired_personal_key", "Bearer oauth_access_token", "Bearer " + testToken + "x"} {
				r := httptest.NewRequest(method, path+"?token="+testToken, nil)
				r.Header.Set("Authorization", auth)
				r.Header.Set("X-ATHub-User", "previously-verified-user")
				r.Header.Set("X-API-Key", testToken)
				r.AddCookie(&http.Cookie{Name: "session", Value: testToken})
				w := httptest.NewRecorder()
				testHandler(t, nil).ServeHTTP(w, r)
				if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") != "Bearer" {
					t.Fatalf("%s %s: alternative credentials accepted: %d", method, path, w.Code)
				}
			}
		}
	}
}
