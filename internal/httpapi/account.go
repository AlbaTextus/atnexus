package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) accountActor(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	w.Header().Set("Cache-Control", "no-store")
	user, ok := a.currentUser(r)
	if !ok {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return user, false
	}
	if r.Method != http.MethodGet && (r.Header.Get("X-AlbaTextus-Client") != "account-center" || r.Header.Get("Sec-Fetch-Site") == "cross-site") {
		writeJSON(w, 403, map[string]string{"error": "invalid_origin"})
		return user, false
	}
	return user, true
}

func (a *API) accountStore(w http.ResponseWriter) (store.AccountStore, bool) {
	st, ok := a.store.(store.AccountStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return st, ok
}

func (a *API) availableApplications(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.accountStore(w)
	if !ok {
		return
	}
	apps, err := st.AvailableApplications(r.Context(), user.Role)
	if err != nil {
		a.applicationError(w, err)
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

func readAccountJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || d.Decode(target) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请提交有效的账户信息。"})
		return false
	}
	return true
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
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
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, userView(updated))
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := a.accountActor(w, r)
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
	hash, err := security.HashPassword(input.NewPassword, a.cfg.BCryptCost)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	err = st.ChangePassword(r.Context(), user.ID, user.PasswordHash, hash)
	if errors.Is(err, store.ErrConflict) {
		writeJSON(w, 409, map[string]string{"error": "account_changed", "error_description": "账户信息已变更，请重新登录后重试。"})
		return
	}
	if err != nil {
		a.applicationError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.cfg.SessionCookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	writeJSON(w, 200, map[string]bool{"reauthenticate": true})
}
