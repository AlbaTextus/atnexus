package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/AlbaTextus/connect/internal/security"
	"github.com/AlbaTextus/connect/internal/store"
	"github.com/go-chi/chi/v5"
)

func isAdmin(user store.User) bool { return user.Role == "ADMIN" || user.Role == "SUPER_ADMIN" }

func (a *API) applicationActor(w http.ResponseWriter, r *http.Request) (store.User, store.ApplicationStore, bool) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return user, nil, false
	}
	if !isAdmin(user) {
		writeJSON(w, 403, map[string]string{"error": "admin_required", "error_description": "应用接入管理仅限管理员使用。"})
		return user, nil, false
	}
	st, ok := a.store.(store.ApplicationStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
		return user, nil, false
	}
	return user, st, true
}

func (a *API) applicationError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrApplicationHasPayments) {
		writeJSON(w, 409, map[string]string{"error": "application_has_payments", "error_description": "应用已有支付记录，请停用应用以保留订单和流水。"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "application_not_found"})
		return
	}
	a.log.Error("application management", "error", err)
	writeJSON(w, 500, map[string]string{"error": "server_error"})
}

func (a *API) listApplications(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	apps, err := st.ListApplications(r.Context(), user.ID, isAdmin(user))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"applications": apps, "can_manage_all": isAdmin(user)})
}

func (a *API) getApplication(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	app, err := st.ApplicationByID(r.Context(), chi.URLParam(r, "id"), user.ID, isAdmin(user))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, app)
}

type applicationInput struct {
	Name         string   `json:"name"`
	HomepageURL  string   `json:"homepage_url"`
	Description  string   `json:"description"`
	IconURL      string   `json:"icon_url"`
	RedirectURIs []string `json:"redirect_uris"`
	MinimumRole  string   `json:"minimum_role"`
	IsActive     *bool    `json:"is_active"`
	IsPublic     bool     `json:"is_public"`
}

func validApplicationURL(value string) bool {
	if len(value) > 2048 || strings.ContainsAny(value, "\\\r\n\t *") {
		return false
	}
	u, err := url.ParseRequestURI(value)
	if err != nil || u.Hostname() == "" || u.User != nil || strings.Contains(value, "#") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

func readApplication(w http.ResponseWriter, r *http.Request) (store.Application, bool) {
	var in applicationInput
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || d.Decode(&in) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请提交有效的应用配置信息。"})
		return store.Application{}, false
	}
	in.Name = strings.TrimSpace(in.Name)
	in.HomepageURL = strings.TrimSpace(in.HomepageURL)
	in.IconURL = strings.TrimSpace(in.IconURL)
	in.Description = strings.TrimSpace(in.Description)
	message := ""
	switch {
	case len(in.Name) == 0 || utf8.RuneCountInString(in.Name) > 100:
		message = "应用名称不能为空，且不能超过 100 个字符。"
	case utf8.RuneCountInString(in.Description) > 1000:
		message = "应用描述过长，请缩短后重试。"
	case !validApplicationURL(in.HomepageURL):
		message = "应用主页需为 HTTPS 地址；本地开发可使用 localhost 或回环 IP 的 HTTP 地址。"
	case in.IconURL != "" && !validApplicationURL(in.IconURL):
		message = "请填写有效的应用图标 HTTPS 地址。"
	case in.MinimumRole != "USER" && in.MinimumRole != "ADMIN" && in.MinimumRole != "SUPER_ADMIN":
		message = "请选择有效的最低访问角色。"
	case len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > 10:
		message = "请填写 1–10 个回调地址。"
	}
	redirects := make([]string, 0, len(in.RedirectURIs))
	seen := map[string]bool{}
	for _, raw := range in.RedirectURIs {
		value := strings.TrimSpace(raw)
		if !validApplicationURL(value) {
			message = "回调地址需为完整 HTTPS 地址，不支持通配符或锚点；本地开发可使用回环 HTTP 地址。"
			break
		}
		if !seen[value] {
			redirects = append(redirects, value)
			seen[value] = true
		}
	}
	if message != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": message})
		return store.Application{}, false
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	return store.Application{Name: in.Name, HomepageURL: in.HomepageURL, Description: in.Description, IconURL: in.IconURL, MinimumRole: in.MinimumRole, RedirectURIs: redirects, IsActive: active, IsPublic: in.IsPublic}, true
}

func (a *API) createApplication(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	app, ok := readApplication(w, r)
	if !ok {
		return
	}
	id, err := security.RandomToken(24)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	app.ID = "at_" + id
	app.OwnerID = &user.ID
	app.AllowedScopes = []string{"profile", "email"}
	app.AllowedGrantTypes = []string{"authorization_code", "refresh_token"}
	var hash *string
	var encryptedSecret *string
	secret := ""
	if !app.IsPublic {
		secret, err = security.RandomToken(32)
		if err != nil {
			a.applicationError(w, err)
			return
		}
		h, err := security.HashPassword(secret, a.cfg.BCryptCost)
		if err != nil {
			a.applicationError(w, err)
			return
		}
		hash = &h
		encrypted, err := a.clientSecrets.Encrypt(app.ID, secret)
		if err != nil {
			a.applicationError(w, err)
			return
		}
		encryptedSecret = &encrypted
	}
	created, err := st.CreateApplication(r.Context(), app, hash, encryptedSecret)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/applications/"+created.ID)
	writeJSON(w, 201, map[string]any{"application": created, "client_secret": secret})
}

func (a *API) updateApplication(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	app, ok := readApplication(w, r)
	if !ok {
		return
	}
	app.ID = chi.URLParam(r, "id")
	previous, err := st.ApplicationByID(r.Context(), app.ID, user.ID, isAdmin(user))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	if previous.IsPublic != app.IsPublic {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "应用类型不能修改，请新建对应类型的应用。"})
		return
	}
	updated, err := st.UpdateApplication(r.Context(), app, user.ID, isAdmin(user))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, updated)
}

func (a *API) deleteApplication(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	if err := st.DeleteApplication(r.Context(), chi.URLParam(r, "id"), user.ID, isAdmin(user)); err != nil {
		a.applicationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getApplicationSecret(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	if r.Header.Get("X-AlbaTextus-Client") != "account-center" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeJSON(w, 403, map[string]string{"error": "invalid_origin"})
		return
	}
	app, err := st.ApplicationByID(r.Context(), chi.URLParam(r, "id"), user.ID, isAdmin(user))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	if app.IsPublic {
		writeJSON(w, 400, map[string]string{"error": "public_client", "error_description": "公开客户端使用 PKCE，不需要 Client Secret。"})
		return
	}
	encrypted, err := st.ApplicationSecret(r.Context(), app.ID, user.ID, isAdmin(user))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	if encrypted == nil || *encrypted == "" {
		writeJSON(w, 409, map[string]string{"error": "secret_unavailable", "error_description": "旧密钥未保存可查看的副本。重置后，新密钥可随时查看；请同步更新子系统配置。"})
		return
	}
	secret, err := a.clientSecrets.Decrypt(app.ID, *encrypted)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"client_secret": secret})
}

func (a *API) rotateApplicationSecret(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.applicationActor(w, r)
	if !ok {
		return
	}
	app, err := st.ApplicationByID(r.Context(), chi.URLParam(r, "id"), user.ID, isAdmin(user))
	if err != nil {
		a.applicationError(w, err)
		return
	}
	if app.IsPublic {
		writeJSON(w, 400, map[string]string{"error": "public_client", "error_description": "公开客户端使用 PKCE，不需要 Client Secret。"})
		return
	}
	secret, err := security.RandomToken(32)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	hash, err := security.HashPassword(secret, a.cfg.BCryptCost)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	encrypted, err := a.clientSecrets.Encrypt(app.ID, secret)
	if err != nil {
		a.applicationError(w, err)
		return
	}
	if err := st.RotateApplicationSecret(r.Context(), app.ID, user.ID, isAdmin(user), hash, encrypted); err != nil {
		a.applicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"client_secret": secret})
}
