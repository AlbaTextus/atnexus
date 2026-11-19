package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRetiredMessageRoutes(t *testing.T) {
	router := (&API{}).Router().(chi.Routes)
	routes := make(map[string]bool)
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method+" "+route] = true
		if strings.HasPrefix(route, "/api/v1/announcements") || strings.HasPrefix(route, "/api/v1/admin/communications") {
			t.Errorf("retired message route remains registered: %s %s", method, route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !routes["POST /api/v1/users"] {
		t.Error("registration route was removed: POST /api/v1/users")
	}
	// Verification codes moved to the mail module under /api/v3.
	for _, route := range []string{
		"POST /api/v1/auth/email/code",
		"POST /api/v1/account/email/code",
		"POST /api/v1/account/email",
	} {
		if routes[route] {
			t.Errorf("retired email route remains registered: %s", route)
		}
	}
}
