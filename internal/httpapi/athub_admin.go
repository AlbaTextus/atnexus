package httpapi

import (
	"net/http"
	"time"

	"github.com/AlbaTextus/oauth/internal/store"
)

func (a *API) managedATHubKey(w http.ResponseWriter, r *http.Request) {
	a.manageATHubKeyAccess(w, r, false)
}
func (a *API) setManagedATHubKey(w http.ResponseWriter, r *http.Request) {
	a.manageATHubKeyAccess(w, r, true)
}
func (a *API) manageATHubKeyAccess(w http.ResponseWriter, r *http.Request, update bool) {
	actor, _, ok := a.userManagementActor(w, r)
	if !ok {
		return
	}
	id, ok := managedUserID(w, r)
	if !ok {
		return
	}
	st, ok := a.store.(store.ATHubAdminStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return
	}
	var result store.ManagedATHubKey
	var err error
	if update {
		var input struct {
			Disabled  *bool     `json:"disabled"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if !readAccountJSON(w, r, &input) {
			return
		}
		if input.Disabled == nil || input.UpdatedAt.IsZero() {
			writeJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		result, err = st.SetManagedATHubKeyDisabled(r.Context(), actor.ID, id, input.UpdatedAt, *input.Disabled)
	} else {
		result, err = st.ManagedATHubKey(r.Context(), actor.ID, id)
	}
	if err != nil {
		a.userManagementError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, result)
}
