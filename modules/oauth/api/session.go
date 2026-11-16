package httpapi

import (
	"errors"
	"net/http"

	"github.com/AlbaTextus/atnexus/modules/oauth"
	"github.com/AlbaTextus/atnexus/pkg/auth"
)

// sessionInfo exposes cache scoping and expiry, never the session credential.
func (a *API) sessionInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, err := oauth.NewSessions(a.Store, a.Config.SessionCookieName).VerifySession(r)
	if err != nil {
		status, code := http.StatusServiceUnavailable, "authentication_unavailable"
		if errors.Is(err, auth.ErrUnauthenticated) {
			status, code = http.StatusUnauthorized, "login_required"
		}
		writeJSON(w, status, map[string]string{"error": code})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cache_scope": identity.SessionID, "expires_at": identity.ExpiresAt})
}
