package api

import (
	"net/http"
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
				w := request(handler, method, path, "{}")
				if w.Code != http.StatusNotFound {
					t.Fatalf("got %d, want 404: %s", w.Code, w.Body.String())
				}
			})
		}
	}
}
