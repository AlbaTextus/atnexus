package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"
)

func (a *API) loginProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	features, err := a.siteFeatures(r.Context())
	if err != nil {
		a.Log.Error("read site features for login providers", "error", err)
		siteFeaturesUnavailable(w)
		return
	}
	providers := make([]map[string]any, 0)
	for _, p := range a.federation.Providers() {
		if p.ID == "custom" && !p.Enabled() {
			continue
		}
		providers = append(providers, map[string]any{"id": p.ID, "name": p.Name, "enabled": features.ExternalLoginEnabled && features.LoginProviderEnabled(p.ID) && p.Enabled()})
	}
	writeJSON(w, 200, map[string]any{"providers": providers, "enabled": features.ExternalLoginEnabled})
}

func (a *API) externalStore(w http.ResponseWriter) (store.ExternalLoginStore, bool) {
	st, ok := a.Store.(store.ExternalLoginStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return st, ok
}

func safeLoginReturn(value string) string {
	if len(value) > 4096 || strings.ContainsAny(value, "\\\r\n\t") || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/account"
	}
	u, err := url.ParseRequestURI(value)
	if err != nil || u.IsAbs() || u.Host != "" || strings.ContainsAny(u.Path, "\\\r\n\t") {
		return "/account"
	}
	if u.Path == "/account" || strings.HasPrefix(u.Path, "/account/") || u.Path == "/admin" || strings.HasPrefix(u.Path, "/admin/") || u.Path == "/oauth/authorize" {
		return u.RequestURI()
	}
	return "/account"
}

func (a *API) startExternalLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("X-AlbaTextus-Client") != "account-center" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeJSON(w, 403, map[string]string{"error": "invalid_origin"})
		return
	}
	features, err := a.siteFeatures(r.Context())
	if err != nil {
		a.Log.Error("read site features for external login", "error", err)
		siteFeaturesUnavailable(w)
		return
	}
	if !features.ExternalLoginEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "external_login_disabled", "error_description": "第三方登录已关闭，请使用邮箱或通行密钥登录。"})
		return
	}
	providerID := chi.URLParam(r, "provider")
	if !features.LoginProviderEnabled(providerID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "login_provider_disabled", "error_description": "该第三方登录方式已关闭，请选择其他登录方式。"})
		return
	}
	provider, ok := a.federation.Provider(providerID)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "provider_unavailable", "error_description": "该登录方式尚未配置，请使用邮箱登录或联系管理员。"})
		return
	}
	var input struct {
		ReturnTo string `json:"return_to"`
		Link     bool   `json:"link"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if !input.Link && !a.verifyHuman(w, r) {
		return
	}
	st, ok := a.externalStore(w)
	if !ok {
		return
	}
	state, err := security.RandomToken(32)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	browser, err := security.RandomToken(32)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	verifier := oauth2.GenerateVerifier()
	flow := store.ExternalLoginFlow{StateHash: security.HashToken(state), BrowserHash: security.HashToken(browser), Provider: provider.ID, ReturnTo: safeLoginReturn(input.ReturnTo), ExpiresAt: time.Now().Add(10 * time.Minute)}
	if input.Link {
		user, ok := a.AccountActor(w, r)
		if !ok {
			return
		}
		if !a.allowMaintenanceLogin(w, r, user) {
			return
		}
		cookie, _ := r.Cookie(a.Config.SessionCookieName)
		flow.UserID = &user.ID
		flow.SessionHash = security.HashToken(cookie.Value)
		flow.ReturnTo = "/account/security"
	}
	flow.VerifierCiphertext, err = a.clientSecrets.Encrypt("login-state:"+flow.StateHash, verifier)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	if err := st.CreateExternalLoginFlow(r.Context(), flow); err != nil {
		a.ApplicationError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "albatextus_oauth_" + provider.ID, Value: browser, Path: "/api/v1/auth/providers/" + provider.ID + "/callback", HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600, Expires: flow.ExpiresAt})
	writeJSON(w, 200, map[string]string{"authorization_url": a.federation.AuthorizationURL(provider, state, verifier)})
}

func (a *API) externalLoginError(w http.ResponseWriter, r *http.Request, flow store.ExternalLoginFlow, code string) {
	path := "/login"
	if maintenance, err := a.maintenanceState(r.Context()); err != nil || maintenance.Enabled {
		path = "/maintenance/login"
	}
	query := url.Values{"oauth_error": {code}}
	if flow.UserID != nil {
		path = "/account/security"
	} else if flow.ReturnTo != "" {
		query.Set("return_to", safeLoginReturn(flow.ReturnTo))
	}
	http.Redirect(w, r, a.Config.AuthUIURL+path+"?"+query.Encode(), http.StatusSeeOther)
}

func (a *API) externalLoginCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var flow store.ExternalLoginFlow
	provider, ok := a.federation.Provider(chi.URLParam(r, "provider"))
	if !ok {
		a.externalLoginError(w, r, flow, "provider_unavailable")
		return
	}
	cookieName := "albatextus_oauth_" + provider.ID
	callbackPath := "/api/v1/auth/providers/" + provider.ID + "/callback"
	browser, err := r.Cookie(cookieName)
	state := r.URL.Query().Get("state")
	if err != nil || len(state) != 43 || len(browser.Value) != 43 {
		a.externalLoginError(w, r, flow, "invalid_state")
		return
	}
	st, ok := a.externalStore(w)
	if !ok {
		return
	}
	flow, err = st.ConsumeExternalLoginFlow(r.Context(), security.HashToken(state), security.HashToken(browser.Value), provider.ID)
	if err != nil {
		a.externalLoginError(w, r, store.ExternalLoginFlow{}, "invalid_state")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: callbackPath, MaxAge: -1, HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode})
	features, err := a.siteFeatures(r.Context())
	if err != nil {
		a.Log.Error("read site features for external login callback", "error", err)
		a.externalLoginError(w, r, flow, "site_features_unavailable")
		return
	}
	if !features.ExternalLoginEnabled {
		a.externalLoginError(w, r, flow, "external_login_disabled")
		return
	}
	if !features.LoginProviderEnabled(provider.ID) {
		a.externalLoginError(w, r, flow, "login_provider_disabled")
		return
	}
	if flow.UserID != nil {
		user, ok := a.CurrentUser(r)
		session, err := r.Cookie(a.Config.SessionCookieName)
		if !ok || err != nil || user.ID != *flow.UserID || security.HashToken(session.Value) != flow.SessionHash {
			a.externalLoginError(w, r, flow, "link_session_expired")
			return
		}
	}
	if r.URL.Query().Get("error") != "" {
		a.externalLoginError(w, r, flow, "authorization_cancelled")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 4096 {
		a.externalLoginError(w, r, flow, "login_failed")
		return
	}
	verifier, err := a.clientSecrets.Decrypt("login-state:"+flow.StateHash, flow.VerifierCiphertext)
	if err != nil {
		a.externalLoginError(w, r, flow, "login_failed")
		return
	}
	identity, err := a.federation.Exchange(r.Context(), provider, code, verifier)
	if err != nil {
		a.externalLoginError(w, r, flow, "provider_failed")
		return
	}
	if flow.UserID != nil {
		user, ok := a.CurrentUser(r)
		if !ok || user.ID != *flow.UserID {
			a.externalLoginError(w, r, flow, "link_session_expired")
			return
		}
	}
	if !a.maintenanceIdentityAllowed(r.Context(), identity, flow.UserID) {
		http.Redirect(w, r, a.Config.AuthUIURL+"/maintenance", http.StatusSeeOther)
		return
	}
	user, err := st.ResolveExternalIdentity(r.Context(), identity, flow.UserID)
	if err != nil {
		message := "login_failed"
		switch {
		case errors.Is(err, store.ErrEmailLinkRequired):
			message = "email_link_required"
		case errors.Is(err, store.ErrVerifiedEmailRequired):
			message = "verified_email_required"
		case errors.Is(err, store.ErrIdentityLinked):
			message = "identity_linked"
		case errors.Is(err, store.ErrInactiveUser):
			message = "account_disabled"
		case errors.Is(err, store.ErrRegistrationDisabled):
			message = "registration_disabled"
		case errors.Is(err, store.ErrExternalLoginDisabled):
			message = "external_login_disabled"
		case errors.Is(err, store.ErrLoginProviderDisabled):
			message = "login_provider_disabled"
		}
		a.externalLoginError(w, r, flow, message)
		return
	}
	if flow.UserID != nil {
		http.Redirect(w, r, a.Config.AuthUIURL+"/account/security?oauth_linked=1", http.StatusSeeOther)
		return
	}
	maintenance, stateErr := a.maintenanceState(r.Context())
	if stateErr != nil || (maintenance.Enabled && user.Role != "SUPER_ADMIN") {
		http.Redirect(w, r, a.Config.AuthUIURL+"/maintenance", http.StatusSeeOther)
		return
	}
	if err := a.issueSession(w, r, user); err != nil {
		a.externalLoginError(w, r, flow, "login_failed")
		return
	}
	destination := safeLoginReturn(flow.ReturnTo)
	if user.ProfileSetupRequired {
		destination = "/welcome?return_to=" + url.QueryEscape(destination)
	}
	http.Redirect(w, r, a.Config.AuthUIURL+destination, http.StatusSeeOther)
}

func (a *API) externalIdentities(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.externalStore(w)
	if !ok {
		return
	}
	identities, err := st.ExternalIdentities(r.Context(), user.ID)
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"identities": identities, "has_password": user.PasswordHash != ""})
}

func (a *API) unlinkExternalIdentity(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.externalStore(w)
	if !ok {
		return
	}
	err := st.UnlinkExternalIdentity(r.Context(), user.ID, chi.URLParam(r, "provider"))
	if errors.Is(err, store.ErrPasswordRequired) {
		writeJSON(w, 409, map[string]string{"error": "password_required", "error_description": "请先设置登录密码，再解绑第三方账号。"})
		return
	}
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
