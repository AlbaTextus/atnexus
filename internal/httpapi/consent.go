package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/AlbaTextus/oauth/internal/oauth"
	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
)

func (a *API) consentStore(w http.ResponseWriter) (store.ConsentStore, bool) {
	st, ok := a.store.(store.ConsentStore)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "service_unavailable"})
	}
	return st, ok
}

func (a *API) beginConsent(w http.ResponseWriter, r *http.Request, user store.User, client store.Client, in oauth.AuthorizeRequest) {
	st, ok := a.consentStore(w)
	if !ok {
		return
	}
	if len(r.URL.RawQuery) > 8192 {
		writeOAuthError(w, oauthErr("invalid_request", "authorization request is too long", 400))
		return
	}
	token, err := security.RandomToken(32)
	if err != nil {
		a.consentError(w, err)
		return
	}
	cookie, _ := r.Cookie(a.cfg.SessionCookieName)
	data, err := json.Marshal(in)
	if err != nil {
		a.consentError(w, err)
		return
	}
	err = st.CreateConsentRequest(r.Context(), store.ConsentRequest{TokenHash: security.HashToken(token), UserID: user.ID, ClientID: client.ID, SessionHash: security.HashToken(cookie.Value), RequestData: data, ClientVersion: client.UpdatedAt, ExpiresAt: time.Now().Add(10 * time.Minute)})
	if err != nil {
		a.consentError(w, err)
		return
	}
	http.Redirect(w, r, a.cfg.AuthUIURL+"/consent?request="+url.QueryEscape(token), http.StatusFound)
}

func (a *API) consentActor(w http.ResponseWriter, r *http.Request) (store.User, string, bool) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return user, "", false
	}
	// Do not trust a supplied Origin from another site, including same-site subdomains.
	if r.Method != http.MethodGet && r.Header.Get("Origin") != "" {
		base, _ := url.Parse(a.cfg.AuthUIURL)
		if base == nil || r.Header.Get("Origin") != base.Scheme+"://"+base.Host {
			writeJSON(w, 403, map[string]string{"error": "invalid_origin"})
			return user, "", false
		}
	}
	cookie, _ := r.Cookie(a.cfg.SessionCookieName)
	return user, security.HashToken(cookie.Value), true
}

func (a *API) consentRequest(w http.ResponseWriter, r *http.Request, token string) (store.ConsentRequest, store.User, bool) {
	user, sessionHash, ok := a.consentActor(w, r)
	if !ok {
		return store.ConsentRequest{}, user, false
	}
	if len(token) != 43 {
		a.consentError(w, store.ErrNotFound)
		return store.ConsentRequest{}, user, false
	}
	st, ok := a.consentStore(w)
	if !ok {
		return store.ConsentRequest{}, user, false
	}
	request, err := st.ReadConsentRequest(r.Context(), security.HashToken(token), user.ID, sessionHash)
	if err != nil {
		a.consentError(w, err)
		return request, user, false
	}
	return request, user, true
}

func (a *API) validateConsentRequest(w http.ResponseWriter, r *http.Request, request store.ConsentRequest) (oauth.AuthorizeRequest, store.Client, bool) {
	var in oauth.AuthorizeRequest
	if json.Unmarshal(request.RequestData, &in) != nil || in.ClientID != request.ClientID {
		a.consentError(w, store.ErrNotFound)
		return in, store.Client{}, false
	}
	client, err := a.oauth.ValidateConsent(r.Context(), request.UserID, in)
	if err != nil {
		writeOAuthError(w, err)
		return in, client, false
	}
	if !request.ClientVersion.Equal(client.UpdatedAt) {
		writeJSON(w, 409, map[string]string{"error": "consent_changed", "error_description": "应用配置已更新，请返回应用重新发起登录。"})
		return in, client, false
	}
	return in, client, true
}

func (a *API) getConsent(w http.ResponseWriter, r *http.Request) {
	request, user, ok := a.consentRequest(w, r, r.URL.Query().Get("request"))
	if !ok {
		return
	}
	in, client, ok := a.validateConsentRequest(w, r, request)
	if !ok {
		return
	}
	homepage, icon := client.HomepageURL, client.IconURL
	if !validApplicationURL(homepage) {
		homepage = ""
	}
	if !validApplicationURL(icon) {
		icon = ""
	}
	callback, _ := url.Parse(in.RedirectURI)
	scopes := strings.Fields(in.Scope)
	slices.Sort(scopes)
	scopes = slices.Compact(scopes)
	minimum := client.MinimumRole
	if minimum == "" {
		minimum = "USER"
	}
	writeJSON(w, 200, map[string]any{
		"application": map[string]any{"id": client.ID, "name": client.Name, "description": client.Description, "homepage_url": homepage, "icon_url": icon, "minimum_role": minimum},
		"user":        map[string]any{"id": user.ID, "email": user.Email, "display_name": user.DisplayName, "avatar_url": user.AvatarURL, "email_is_alias": strings.HasSuffix(user.Email, "@accounts.albatextus.invalid")},
		"scopes":      scopes, "can_refresh": slices.Contains(client.AllowedGrantTypes, "refresh_token"), "redirect_origin": callback.Scheme + "://" + callback.Host, "expires_at": request.ExpiresAt,
	})
}

func (a *API) decideConsent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, _, ok := a.consentActor(w, r); !ok {
		return
	}
	var input struct {
		RequestToken string `json:"request_token"`
		Decision     string `json:"decision"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if input.Decision != "allow" && input.Decision != "deny" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request", "error_description": "请选择允许或拒绝授权。"})
		return
	}
	request, _, ok := a.consentRequest(w, r, input.RequestToken)
	if !ok {
		return
	}
	in, _, ok := a.validateConsentRequest(w, r, request)
	if !ok {
		return
	}
	destination, _ := url.Parse(in.RedirectURI)
	query := destination.Query()
	query.Del("code")
	query.Del("error")
	query.Del("error_description")
	query.Del("state")
	var prepared *store.NewAuthorizationCode
	if input.Decision == "deny" {
		query.Set("error", "access_denied")
		query.Set("error_description", "The user denied the authorization request.")
	} else {
		code, record, err := a.oauth.PrepareAuthorizationCode(r.Context(), request.UserID, in)
		if err != nil {
			writeOAuthError(w, err)
			return
		}
		query.Set("code", code)
		prepared = &record
	}
	st, ok := a.consentStore(w)
	if !ok {
		return
	}
	if err := st.CompleteConsentRequest(r.Context(), request, prepared); err != nil {
		a.consentError(w, err)
		return
	}
	if in.State != "" {
		query.Set("state", in.State)
	}
	destination.RawQuery = query.Encode()
	writeJSON(w, 200, map[string]string{"redirect_url": destination.String()})
}

func (a *API) consentError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConsentChanged) {
		writeJSON(w, 409, map[string]string{"error": "consent_changed", "error_description": "账户权限或应用配置已更新，请返回应用重新发起登录。"})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, 400, map[string]string{"error": "invalid_consent", "error_description": "授权请求已过期、已处理或不属于当前登录账户，请返回应用重新发起登录。"})
		return
	}
	a.log.Error("OAuth consent operation failed", "error", err)
	writeJSON(w, 500, map[string]string{"error": "server_error"})
}

// Subsystems receive only the user claims covered by their approved scopes.
func consentUserInfo(user store.User, scopes []string) map[string]any {
	view := map[string]any{"id": user.ID, "sub": user.ID}
	if slices.Contains(scopes, "profile") {
		view["numeric_id"] = user.NumericID
		view["display_name"] = user.DisplayName
		view["role"] = user.Role
		view["created_at"] = user.CreatedAt
		view["avatar_url"] = user.AvatarURL
		view["picture"] = user.AvatarURL
	}
	if slices.Contains(scopes, "email") {
		view["email"] = user.Email
		view["email_verified"] = user.EmailVerified
		view["email_is_alias"] = strings.HasSuffix(user.Email, "@accounts.albatextus.invalid")
	}
	return view
}
