package httpapi

import (
	"net/http"

	"github.com/google/uuid"
)

func (a *API) SetATHubSubmitter(submit func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID)) {
	a.athubSubmitter = submit
}

func (a *API) athubSubmit(w http.ResponseWriter, r *http.Request) {
	user, session, ok := a.ATHubSessionActor(w, r)
	if !ok {
		return
	}
	if a.athubSubmitter == nil {
		writeJSON(w, 503, map[string]string{"error": "submission_unavailable", "error_description": "提交服务暂时不可用，请稍后重试。"})
		return
	}
	a.athubSubmitter(w, r, user.ID, session.ID)
}
