package httpapi

import (
	"net/http"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/google/uuid"
)

func (a *API) SetATHubSubmitter(submit func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID)) {
	a.athubSubmitter = submit
}

func (a *API) athubSubmit(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.ATHubActor(w, r)
	if !ok {
		return
	}
	cookie, err := r.Cookie(a.Config.SessionCookieName)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	session, err := a.Store.SessionByHash(r.Context(), security.HashToken(cookie.Value))
	if err != nil || session.UserID != user.ID || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return
	}
	if a.athubSubmitter == nil {
		writeJSON(w, 503, map[string]string{"error": "submission_unavailable", "error_description": "提交服务暂时不可用，请稍后重试。"})
		return
	}
	a.athubSubmitter(w, r, user.ID, session.ID)
}
