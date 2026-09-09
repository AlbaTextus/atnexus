package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlbaTextus/connect/internal/security"
	"github.com/AlbaTextus/connect/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) userManagementActor(w http.ResponseWriter, r *http.Request) (store.User, store.UserManagementStore, bool) {
	u, ok := a.accountActor(w, r)
	if !ok {
		return u, nil, false
	}
	if !isAdmin(u) {
		writeJSON(w, 403, map[string]string{"error": "admin_required", "error_description": "用户管理仅限管理员使用。"})
		return u, nil, false
	}
	st, ok := a.store.(store.UserManagementStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return u, st, ok
}

func (a *API) userManagementError(w http.ResponseWriter, err error) {
	status, code, message := 500, "server_error", "用户管理暂时不可用，请稍后重试。"
	switch {
	case errors.Is(err, store.ErrNotFound):
		status, code, message = 404, "user_not_found", "用户不存在，请刷新列表。"
	case errors.Is(err, store.ErrAlreadyExists):
		status, code, message = 409, "email_exists", "该邮箱已被使用。"
	case errors.Is(err, store.ErrConflict):
		status, code, message = 409, "user_conflict", "用户信息已变更，请关闭窗口并刷新列表后重试。"
	case errors.Is(err, store.ErrUserForbidden):
		status, code, message = 403, "user_forbidden", "无权操作此用户或分配该角色；不能修改自己的角色、停用自己或重置自己的登录。"
	default:
		a.log.Error("user management", "error", err)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": message})
}

func managedUserView(u store.ManagedUser) map[string]any {
	view := userView(u.User)
	view["is_active"], view["updated_at"] = u.IsActive, u.UpdatedAt
	return view
}

func validUserRole(role string) bool {
	return role == "USER" || role == "ADMIN" || role == "SUPER_ADMIN"
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	_, st, ok := a.userManagementActor(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	filter := store.UserFilter{Search: strings.TrimSpace(q.Get("q")), Role: q.Get("role"), Status: q.Get("status"), Page: 1, PageSize: 20}
	valid := true
	for key, target := range map[string]*int{"page": &filter.Page, "page_size": &filter.PageSize} {
		if q.Has(key) {
			value, err := strconv.Atoi(q.Get(key))
			if err != nil {
				valid = false
			}
			*target = value
		}
	}
	if !valid || filter.Page < 1 || filter.Page > 1000000 || filter.PageSize < 1 || filter.PageSize > 100 || utf8.RuneCountInString(filter.Search) > 320 || (filter.Role != "" && !validUserRole(filter.Role)) || (filter.Status != "" && filter.Status != "active" && filter.Status != "inactive") {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "搜索条件或分页参数无效。"})
		return
	}
	result, err := st.ListUsers(r.Context(), filter)
	if err != nil {
		a.userManagementError(w, err)
		return
	}
	users := make([]map[string]any, 0, len(result.Users))
	for _, u := range result.Users {
		users = append(users, managedUserView(u))
	}
	writeJSON(w, 200, map[string]any{"users": users, "total": result.Total, "page": filter.Page, "page_size": filter.PageSize, "stats": map[string]int{"total": result.All, "active": result.Active, "admins": result.Admins}})
}

func (a *API) createManagedUser(w http.ResponseWriter, r *http.Request) {
	actor, st, ok := a.userManagementActor(w, r)
	if !ok {
		return
	}
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if !readAccountJSON(w, r, &in) {
		return
	}
	in.Email, in.DisplayName = normalizeEmail(in.Email), strings.TrimSpace(in.DisplayName)
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || !strings.Contains(in.Email, "@") || strings.HasSuffix(in.Email, ".invalid") || len(in.Email) > 320 || len(in.Password) < 12 || len(in.Password) > 72 || utf8.RuneCountInString(in.DisplayName) > 100 || !validUserRole(in.Role) {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请填写有效邮箱、12–72 字节的密码、最多 100 字符的昵称及有效角色。"})
		return
	}
	if actor.Role != "SUPER_ADMIN" && in.Role != "USER" {
		a.userManagementError(w, store.ErrUserForbidden)
		return
	}
	hash, err := security.HashPassword(in.Password, a.cfg.BCryptCost)
	if err != nil {
		a.userManagementError(w, err)
		return
	}
	var name *string
	if in.DisplayName != "" {
		name = &in.DisplayName
	}
	u, err := st.CreateManagedUser(r.Context(), actor.ID, store.UserSeed{Email: in.Email, PasswordHash: hash, DisplayName: name, Role: in.Role})
	if err != nil {
		a.userManagementError(w, err)
		return
	}
	writeJSON(w, 201, managedUserView(u))
}

func managedUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || id == uuid.Nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "用户 ID 无效。"})
		return uuid.Nil, false
	}
	return id, true
}

func (a *API) updateManagedUser(w http.ResponseWriter, r *http.Request) {
	actor, st, ok := a.userManagementActor(w, r)
	if !ok {
		return
	}
	id, ok := managedUserID(w, r)
	if !ok {
		return
	}
	var in struct {
		DisplayName *string   `json:"display_name"`
		Role        string    `json:"role"`
		IsActive    *bool     `json:"is_active"`
		UpdatedAt   time.Time `json:"updated_at"`
	}
	if !readAccountJSON(w, r, &in) {
		return
	}
	if in.DisplayName == nil || utf8.RuneCountInString(strings.TrimSpace(*in.DisplayName)) > 100 || !validUserRole(in.Role) || in.IsActive == nil || in.UpdatedAt.IsZero() {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请提交完整的昵称、角色、状态与用户版本。"})
		return
	}
	name := strings.TrimSpace(*in.DisplayName)
	var display *string
	if name != "" {
		display = &name
	}
	u, err := st.UpdateManagedUser(r.Context(), actor.ID, id, store.UserChange{DisplayName: display, Role: in.Role, IsActive: *in.IsActive, UpdatedAt: in.UpdatedAt})
	if err != nil {
		a.userManagementError(w, err)
		return
	}
	writeJSON(w, 200, managedUserView(u))
}

func (a *API) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	a.changeManagedUserAccess(w, r, true)
}
func (a *API) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	a.changeManagedUserAccess(w, r, false)
}

func (a *API) changeManagedUserAccess(w http.ResponseWriter, r *http.Request, password bool) {
	actor, st, ok := a.userManagementActor(w, r)
	if !ok {
		return
	}
	id, ok := managedUserID(w, r)
	if !ok {
		return
	}
	var in struct {
		Password  string    `json:"new_password"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if !readAccountJSON(w, r, &in) {
		return
	}
	if in.UpdatedAt.IsZero() || (password && (len(in.Password) < 12 || len(in.Password) > 72)) || (!password && in.Password != "") {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请提交有效的用户版本；新密码需为 12–72 字节。"})
		return
	}
	var err error
	if password {
		hash, hashErr := security.HashPassword(in.Password, a.cfg.BCryptCost)
		if hashErr != nil {
			a.userManagementError(w, hashErr)
			return
		}
		err = st.ResetUserPassword(r.Context(), actor.ID, id, in.UpdatedAt, hash)
	} else {
		err = st.RevokeUserSessions(r.Context(), actor.ID, id, in.UpdatedAt)
	}
	if err != nil {
		a.userManagementError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
