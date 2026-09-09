package httpapi

import (
	"net/http"

	"github.com/AlbaTextus/connect/internal/store"
	"github.com/go-chi/chi/v5"
)

func (a *API) updateApplicationStatus(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	var input struct {
		IsActive *bool `json:"is_active"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if input.IsActive == nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请选择启用或停用状态。"})
		return
	}
	st, ok := a.store.(store.ApplicationStatusStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	app, err := st.SetApplicationStatus(r.Context(), chi.URLParam(r, "id"), user.ID, isAdmin(user), *input.IsActive)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, app)
}
