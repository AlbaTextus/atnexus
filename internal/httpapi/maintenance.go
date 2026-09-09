package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/AlbaTextus/connect/internal/store"
	"github.com/google/uuid"
)

func (a *API) maintenanceState(ctx context.Context) (store.MaintenanceState, error) {
	st, ok := a.store.(store.MaintenanceStore)
	if !ok {
		return store.MaintenanceState{}, errors.New("maintenance store unavailable")
	}
	return st.Maintenance(ctx)
}

func maintenanceReply(w http.ResponseWriter, unavailable bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "15")
	code, message := "maintenance", "网站维护中，目前仅超级管理员可以访问。"
	if unavailable {
		code, message = "maintenance_unavailable", "暂时无法确认服务状态，请稍后重试。"
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": code, "error_description": message})
}

// Keep only the transports required for super-admin authentication open.
// Role validation happens after authentication, before creating any session.
func maintenancePublicRoute(r *http.Request) bool {
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/healthz", "/api/v1/maintenance", "/api/v1/auth/verification", "/api/v1/auth/providers":
			return true
		}
	}
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/api/v1/sessions", "/api/v1/passkeys/login/options", "/api/v1/passkeys/login/finish":
			return true
		}
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "auth" && parts[3] == "providers" && parts[4] != "" {
		return (r.Method == http.MethodPost && parts[5] == "start") || (r.Method == http.MethodGet && parts[5] == "callback")
	}
	return r.Method == http.MethodDelete && r.URL.Path == "/api/v1/sessions/current"
}

func (a *API) maintenanceIdentityAllowed(ctx context.Context, identity store.ExternalIdentity, linkUser *uuid.UUID) bool {
	state, err := a.maintenanceState(ctx)
	if err != nil {
		return false
	}
	if !state.Enabled {
		return true
	}
	var user store.User
	if linkUser != nil {
		user, err = a.store.UserByID(ctx, *linkUser)
	} else {
		st, ok := a.store.(store.MaintenanceIdentityStore)
		if !ok {
			return false
		}
		user, err = st.ExistingExternalUser(ctx, identity.Provider, identity.Subject)
	}
	return err == nil && user.IsActive && user.Role == "SUPER_ADMIN"
}

func (a *API) maintenanceGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maintenancePublicRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		state, err := a.maintenanceState(r.Context())
		if err != nil {
			maintenanceReply(w, true)
			return
		}
		if state.Enabled {
			user, ok := a.currentUser(r)
			if !ok || user.Role != "SUPER_ADMIN" {
				maintenanceReply(w, false)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) allowMaintenanceLogin(w http.ResponseWriter, r *http.Request, user store.User) bool {
	state, err := a.maintenanceState(r.Context())
	if err != nil || (state.Enabled && user.Role != "SUPER_ADMIN") {
		maintenanceReply(w, err != nil)
		return false
	}
	return true
}

func (a *API) getMaintenance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Cookie")
	state, err := a.maintenanceState(r.Context())
	if err != nil {
		maintenanceReply(w, true)
		return
	}
	user, ok := a.currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": state.Enabled, "bypass": ok && user.Role == "SUPER_ADMIN", "updated_at": state.UpdatedAt,
	})
}

func (a *API) setMaintenance(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	if user.Role != "SUPER_ADMIN" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "super_admin_required", "error_description": "仅超级管理员可以设置维护模式。"})
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled_required"})
		return
	}
	st, ok := a.store.(store.MaintenanceStore)
	if !ok {
		maintenanceReply(w, true)
		return
	}
	state, err := st.SetMaintenance(r.Context(), *input.Enabled, user.ID)
	if err != nil {
		maintenanceReply(w, true)
		return
	}
	a.log.Info("maintenance mode updated", "enabled", state.Enabled, "actor", user.ID)
	writeJSON(w, http.StatusOK, state)
}
