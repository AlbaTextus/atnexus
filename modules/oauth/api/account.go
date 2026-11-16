package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) accountStore(w http.ResponseWriter) (store.AccountStore, bool) {
	st, ok := a.Store.(store.AccountStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return st, ok
}

func (a *API) availableApplications(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.accountStore(w)
	if !ok {
		return
	}
	apps, err := st.AvailableApplications(r.Context(), user.Role)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	// Older seeded clients may have missing or unvalidated homepage/icon URLs.
	for i := range apps {
		if !validApplicationURL(apps[i].HomepageURL) {
			apps[i].HomepageURL = ""
		}
		if !validApplicationURL(apps[i].IconURL) {
			apps[i].IconURL = ""
		}
	}
	writeJSON(w, 200, map[string]any{"applications": apps})
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	if !isAdmin(user) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "profile_edit_forbidden", "error_description": "昵称仅限管理员及以上修改。"})
		return
	}
	var input struct {
		DisplayName string `json:"display_name"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	name := strings.TrimSpace(input.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		writeJSON(w, 400, map[string]string{"error": "invalid_display_name", "error_description": "昵称需为 1–100 个字符。"})
		return
	}
	st, ok := a.accountStore(w)
	if !ok {
		return
	}
	updated, err := st.UpdateProfile(r.Context(), user.ID, &name)
	if errors.Is(err, store.ErrUserForbidden) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "profile_edit_forbidden", "error_description": "昵称仅限管理员及以上修改。"})
		return
	}
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if (user.PasswordHash != "" && !security.CheckPassword(user.PasswordHash, input.CurrentPassword)) || (user.PasswordHash == "" && input.CurrentPassword != "") {
		writeJSON(w, 400, map[string]string{"error": "incorrect_password", "error_description": "当前密码不正确。"})
		return
	}
	if len(input.NewPassword) < 12 || len(input.NewPassword) > 72 {
		writeJSON(w, 400, map[string]string{"error": "invalid_password", "error_description": "新密码需为 12–72 字节；中文等字符会占用多个字节。"})
		return
	}
	if input.NewPassword == input.CurrentPassword {
		writeJSON(w, 400, map[string]string{"error": "password_unchanged", "error_description": "新密码不能与当前密码相同。"})
		return
	}
	st, ok := a.accountStore(w)
	if !ok {
		return
	}
	hash, err := security.HashPassword(input.NewPassword, a.Config.BCryptCost)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	err = st.ChangePassword(r.Context(), user.ID, user.PasswordHash, hash)
	if errors.Is(err, store.ErrConflict) {
		writeJSON(w, 409, map[string]string{"error": "account_changed", "error_description": "账户信息已变更，请重新登录后重试。"})
		return
	}
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.Config.SessionCookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode})
	writeJSON(w, 200, map[string]bool{"reauthenticate": true})
}
