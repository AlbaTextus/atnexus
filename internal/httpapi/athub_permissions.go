package httpapi

import (
	"errors"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) athubPolicyError(w http.ResponseWriter, err error) {
	status, code, message := 503, "permissions_unavailable", "暂时无法处理权限或审核，请稍后重试。"
	switch {
	case errors.Is(err, store.ErrUserForbidden), errors.Is(err, store.ErrATHubPermissionDenied):
		status, code, message = 403, "operation_forbidden", "当前账户没有此操作权限，或提交者的权限已发生变化。"
	case errors.Is(err, store.ErrNotFound):
		status, code, message = 404, "review_not_found", "没有找到这项提交。"
	case errors.Is(err, store.ErrConflict):
		status, code, message = 409, "review_conflict", "记录已被处理或配置已更新，请刷新后重试。"
	case errors.Is(err, store.ErrInvalidATHubKeyLimit):
		status, code, message = 400, "invalid_policy", "请提交完整且有效的权限配置。"
	default:
		a.log.Error("ATHub permissions or review failed", "error", err)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": message})
}
func (a *API) athubPermissions(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.store.(store.ATHubPolicyStore)
	if !ok {
		a.athubPolicyError(w, errors.New("policy store unavailable"))
		return
	}
	if r.URL.Path == "/api/v1/account/athub/permissions" {
		policy, err := st.ATHubPolicy(r.Context(), user.Role)
		if err != nil {
			a.athubPolicyError(w, err)
			return
		}
		writeJSON(w, 200, policy)
		return
	}
	if !isAdmin(user) {
		a.athubPolicyError(w, store.ErrUserForbidden)
		return
	}
	if r.Method == http.MethodGet {
		policies, err := st.ATHubPolicies(r.Context())
		if err != nil {
			a.athubPolicyError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"policies": policies, "endpoints": store.ATHubEndpoints()})
		return
	}
	if user.Role != "SUPER_ADMIN" {
		a.athubPolicyError(w, store.ErrUserForbidden)
		return
	}
	var policy store.ATHubRolePolicy
	if !readAccountJSON(w, r, &policy) {
		return
	}
	updated, err := st.SetATHubPolicy(r.Context(), user.ID, policy)
	if err != nil {
		a.athubPolicyError(w, err)
		return
	}
	a.log.Info("ATHub permissions updated", "actor", user.ID, "role", policy.Role)
	writeJSON(w, 200, updated)
}
