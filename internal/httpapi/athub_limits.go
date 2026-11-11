package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) athubKeyLimits(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	if !isAdmin(user) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin_required", "error_description": "Key 调用限制仅限管理员配置。"})
		return
	}
	st, ok := a.store.(store.ATHubKeyLimitStore)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service_unavailable"})
		return
	}
	if r.Method == http.MethodGet {
		limits, err := st.ATHubKeyLimits(r.Context())
		if err != nil {
			a.applicationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"limits": limits})
		return
	}
	var input struct {
		Role           string          `json:"role"`
		TotalCallLimit json.RawMessage `json:"total_call_limit"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	var limit *int64
	if len(input.TotalCallLimit) == 0 || json.Unmarshal(input.TotalCallLimit, &limit) != nil || !store.ValidATHubKeyLimit(input.Role, limit) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_key_limit", "error_description": "请选择已开放的等级；总调用上限需为非负整数或不限次。",
		})
		return
	}
	result, err := st.SetATHubKeyLimit(r.Context(), input.Role, limit, user.ID)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	a.log.Info("ATHub key limit updated", "role", input.Role, "total_call_limit", limit, "actor", user.ID)
	writeJSON(w, http.StatusOK, result)
}
