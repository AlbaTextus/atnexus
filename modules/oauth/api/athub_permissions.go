package httpapi

import (
	"errors"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) athubPermissions(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.Store.(store.ATHubPolicyStore)
	if !ok {
		a.PolicyError(w, errors.New("policy store unavailable"))
		return
	}
	if r.URL.Path == "/api/v1/account/athub/permissions" {
		policy, err := st.ATHubPolicy(r.Context(), user.Role)
		if err != nil {
			a.PolicyError(w, err)
			return
		}
		writeJSON(w, 200, policy)
		return
	}
	if !isAdmin(user) {
		a.PolicyError(w, store.ErrUserForbidden)
		return
	}
	if r.Method == http.MethodGet {
		policies, err := st.ATHubPolicies(r.Context())
		if err != nil {
			a.PolicyError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"policies": policies, "endpoints": store.ATHubEndpoints()})
		return
	}
	if user.Role != "SUPER_ADMIN" {
		a.PolicyError(w, store.ErrUserForbidden)
		return
	}
	var policy store.ATHubRolePolicy
	if !readAccountJSON(w, r, &policy) {
		return
	}
	updated, err := st.SetATHubPolicy(r.Context(), user.ID, policy)
	if err != nil {
		a.PolicyError(w, err)
		return
	}
	a.Log.Info("ATHub permissions updated", "actor", user.ID, "role", policy.Role)
	writeJSON(w, 200, updated)
}
