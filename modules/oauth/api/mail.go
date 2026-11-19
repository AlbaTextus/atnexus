package httpapi

import (
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
)

// The mail module (/api/v3) applies these account rules through its Account
// interface, so the two modules cannot drift apart.

func (a *API) RequireRegistration(w http.ResponseWriter, r *http.Request) bool {
	return a.requireRegistration(w, r)
}

func (a *API) VerifyHuman(w http.ResponseWriter, r *http.Request) bool { return a.verifyHuman(w, r) }

func (a *API) MaintenanceGate(next http.Handler) http.Handler { return a.maintenanceGate(next) }

func (a *API) UserView(user store.User) map[string]any { return userView(user) }
