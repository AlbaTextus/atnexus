package httpapi

import (
	"net/http"

	"github.com/AlbaTextus/atnexus/modules/athub/access"
)

func (a *API) athubReadSession(w http.ResponseWriter, r *http.Request) {
	user, session, ok := a.ATHubSessionActor(w, r)
	if !ok {
		return
	}
	token, expires, err := athubaccess.IssueReadSession(a.Config.ClientSecretEncryptionKey, user.ID, session.ID, session.ExpiresAt)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"access_token": token, "expires_at": expires, "cache_scope": session.ID})
}
