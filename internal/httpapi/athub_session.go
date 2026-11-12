package httpapi

import (
	"net/http"
	"time"

	"github.com/AlbaTextus/atnexus/internal/athubaccess"
	"github.com/AlbaTextus/atnexus/internal/security"
)

func (a *API) athubReadSession(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.athubActor(w, r)
	if !ok {
		return
	}
	cookie, err := r.Cookie(a.cfg.SessionCookieName)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	session, err := a.store.SessionByHash(r.Context(), security.HashToken(cookie.Value))
	if err != nil || session.UserID != user.ID || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	token, expires, err := athubaccess.IssueReadSession(a.cfg.ClientSecretEncryptionKey, user.ID, session.ID, session.ExpiresAt)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"access_token": token, "expires_at": expires, "cache_scope": session.ID})
}
