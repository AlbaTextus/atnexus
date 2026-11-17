package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/oauth/email"
	"github.com/AlbaTextus/atnexus/modules/oauth/federation"
	"github.com/AlbaTextus/atnexus/modules/oauth/passkey"
	"github.com/AlbaTextus/atnexus/modules/oauth/protocol"
	"github.com/AlbaTextus/atnexus/modules/oauth/turnstile"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type API struct {
	*apibase.Base
	oauth         *oauth.Service
	passkey       *passkey.Service
	clientSecrets *security.SecretCipher
	federation    *federation.Service
	turnstile     *turnstile.Service
	mailer        email.Sender
	imageKeys     http.Handler
	decoyOnce     sync.Once
	decoyHash     string
}

func (a *API) SetImageKeys(handler http.Handler) { a.imageKeys = handler }

func New(cfg config.Config, st store.Store, logger *slog.Logger) (*API, error) {
	if logger == nil {
		logger = slog.Default()
	}
	clientSecrets, err := security.NewSecretCipher(cfg.ClientSecretEncryptionKey)
	if err != nil {
		return nil, err
	}
	passkeyService, err := passkey.NewService(st, cfg)
	if err != nil {
		return nil, err
	}
	federatedLogin, err := federation.New(cfg)
	if err != nil {
		return nil, err
	}
	humanVerification, err := turnstile.New(cfg)
	if err != nil {
		return nil, err
	}
	return &API{Base: &apibase.Base{Config: cfg, Store: st, Log: logger}, oauth: oauth.NewService(st, cfg), passkey: passkeyService, clientSecrets: clientSecrets, federation: federatedLogin, turnstile: humanVerification, mailer: email.New(cfg.SMTP)}, nil
}

func (a *API) Router() http.Handler {
	if a.Base == nil {
		a.Base = &apibase.Base{}
	}
	r := chi.NewRouter()
	// The listener wraps this router in the request timeout (server.ModuleHTTP);
	// a second TimeoutHandler here would buffer every response twice.
	r.Use(a.requestID, a.securityHeaders, a.recoverer, a.accessLog, a.maintenanceGate)
	r.Get("/api/v1/maintenance", a.getMaintenance)
	if a.imageKeys != nil {
		r.Handle("/api/v1/admin/image/keys", a.imageKeys)
	}
	r.Put("/api/v1/admin/maintenance", a.setMaintenance)
	r.Get("/api/v1/admin/features", a.getSiteFeatures)
	r.Patch("/api/v1/admin/features", a.setSiteFeature)
	r.Get("/healthz", a.health)
	r.Get("/.well-known/oauth-authorization-server", a.metadata)
	r.Post("/api/v1/users", a.register)
	r.Post("/api/v1/auth/email/code", a.sendRegistrationCode)
	r.Post("/api/v1/account/email/code", a.sendAccountEmailCode)
	r.Post("/api/v1/account/email", a.confirmAccountEmail)
	r.Post("/api/v1/sessions", a.login)
	r.Delete("/api/v1/sessions/current", a.logout)
	r.Get("/api/v1/user", a.me)
	r.Get("/api/v1/profiles/{id}", a.publicProfile)
	r.Get("/api/v1/session", a.sessionInfo)
	r.Get("/api/v1/admin/users", a.listUsers)
	r.Post("/api/v1/admin/users", a.createManagedUser)
	r.Patch("/api/v1/admin/users/{id}", a.updateManagedUser)
	r.Post("/api/v1/admin/users/{id}/password", a.resetUserPassword)
	r.Post("/api/v1/admin/users/{id}/revoke-sessions", a.revokeUserSessions)
	r.Get("/api/v1/admin/users/{id}/athub-key", a.managedATHubKey)
	r.Put("/api/v1/admin/users/{id}/athub-key", a.setManagedATHubKey)
	r.Get("/api/v1/admin/athub/dashboard", a.athubDashboard)
	r.Get("/api/v1/admin/athub/key-limits", a.athubKeyLimits)
	r.Put("/api/v1/admin/athub/key-limits", a.athubKeyLimits)
	r.Get("/api/v1/admin/athub/permissions", a.athubPermissions)
	r.Put("/api/v1/admin/athub/permissions", a.athubPermissions)
	r.Get("/api/v1/account/athub/permissions", a.athubPermissions)
	r.Get("/api/v1/auth/providers", a.loginProviders)
	r.Get("/api/v1/auth/verification", a.authVerification)
	r.Get("/api/v1/legal/{slug}", a.getLegalDocument)
	r.Get("/api/v1/admin/legal/{slug}", a.getAdminLegalDocument)
	r.Put("/api/v1/admin/legal/{slug}", a.updateLegalDocument)
	r.Post("/api/v1/auth/providers/{provider}/start", a.startExternalLogin)
	r.Get("/api/v1/auth/providers/{provider}/callback", a.externalLoginCallback)
	r.Get("/api/v1/account/identities", a.externalIdentities)
	r.Delete("/api/v1/account/identities/{provider}", a.unlinkExternalIdentity)
	r.Get("/api/v1/account/applications", a.availableApplications)
	r.Get("/api/v1/account/key", a.listATHubKeys)
	r.Post("/api/v1/account/key", a.createATHubKey)
	r.Post("/api/v1/account/key/reset", a.resetATHubKey)
	r.Delete("/api/v1/account/key/{id}", a.revokeATHubKey)
	r.Get("/api/v1/account/athub/usage", a.athubUsage)
	r.Get("/api/v1/account/athub/calls", a.athubCalls)
	r.Post("/api/v1/account/athub/session", a.athubReadSession)
	r.Patch("/api/v1/account/profile", a.updateProfile)
	r.Post("/api/v1/account/setup", a.completeProfileSetup)
	r.Patch("/api/v1/account/bio", a.updateBio)
	r.Post("/api/v1/account/background", a.uploadBackground)
	r.Delete("/api/v1/account/background", a.removeBackground)
	r.Patch("/api/v1/account/avatar", a.updateAvatar)
	r.Post("/api/v1/account/avatar", a.uploadAvatar)
	r.Post("/api/v1/account/avatar/preview", a.previewAvatarGIF)
	r.Post("/api/v1/account/password", a.changePassword)
	r.Get("/api/v1/applications", a.listApplications)
	r.Post("/api/v1/applications", a.createApplication)
	r.Get("/api/v1/applications/{id}", a.getApplication)
	r.Put("/api/v1/applications/{id}", a.updateApplication)
	r.Patch("/api/v1/applications/{id}/status", a.updateApplicationStatus)
	r.Delete("/api/v1/applications/{id}", a.deleteApplication)
	r.Post("/api/v1/applications/{id}/secret", a.rotateApplicationSecret)
	r.Get("/api/v1/applications/{id}/secret", a.getApplicationSecret)
	r.Post("/api/v1/passkeys/registration/options", a.passkeyRegistrationOptions)
	r.Post("/api/v1/passkeys/registration/finish", a.passkeyRegistrationFinish)
	r.Post("/api/v1/passkeys/login/options", a.passkeyLoginOptions)
	r.Post("/api/v1/passkeys/login/finish", a.passkeyLoginFinish)
	r.Get("/api/v1/passkeys", a.listPasskeys)
	r.Delete("/api/v1/passkeys/{id}", a.deletePasskey)
	r.Get("/oauth/authorize", a.authorize)
	r.Get("/api/v1/oauth/consent", a.getConsent)
	r.Post("/api/v1/oauth/consent", a.decideConsent)
	r.Post("/oauth/token", a.token)
	r.Post("/oauth/revoke", a.revoke)
	r.Post("/oauth/introspect", a.introspect)
	return r
}

func (a *API) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			var b [12]byte
			if _, err := rand.Read(b[:]); err == nil {
				id = hex.EncodeToString(b[:])
			} else {
				id = "unknown"
			}
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

type requestIDKey struct{}

func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (a *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				a.Log.Error("panic recovered", "panic", value)
				writeJSON(w, 500, map[string]string{"error": "internal_server_error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *API) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		a.Log.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := a.Store.Ping(r.Context()); err != nil {
		writeJSON(w, 503, map[string]string{"status": "unhealthy"})
		return
	}
	state, err := a.maintenanceState(r.Context())
	if err != nil {
		writeJSON(w, 503, map[string]string{"status": "unhealthy"})
		return
	}
	if state.Enabled {
		w.Header().Set("Retry-After", "15")
		writeJSON(w, 433, map[string]string{"status": "maintenance"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *API) metadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"userinfo_endpoint": a.Config.IssuerURL + "/api/v1/user",
		"issuer":            a.Config.IssuerURL, "authorization_endpoint": a.Config.IssuerURL + "/oauth/authorize", "token_endpoint": a.Config.IssuerURL + "/oauth/token", "revocation_endpoint": a.Config.IssuerURL + "/oauth/revoke", "introspection_endpoint": a.Config.IssuerURL + "/oauth/introspect",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token", "client_credentials"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
	})
}

type userInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	EmailCode   string `json:"email_code"`
	ChallengeID string `json:"challenge_id"`
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	if !a.requireRegistration(w, r) {
		return
	}
	if !a.verifyHuman(w, r) {
		return
	}
	var in userInput
	if err := decodeJSON(r, &in); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid_request", "error_description": "request body must be valid JSON"})
		return
	}
	email, password, _, valid := validateUserInput(in)
	if !valid {
		writeJSON(w, 400, map[string]any{"error": "invalid_request", "error_description": "email and a password of 12-72 characters are required"})
		return
	}
	st, ok := a.emailStore(w)
	if !ok {
		return
	}
	id, err := uuid.Parse(in.ChallengeID)
	if err != nil {
		a.emailError(w, store.ErrEmailCode)
		return
	}
	hash, err := security.HashPassword(password, a.Config.BCryptCost)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "server_error"})
		return
	}
	u, err := st.CompleteEmailVerification(r.Context(), store.EmailCompletion{
		EmailChallenge: store.EmailChallenge{ID: id, Purpose: "register", Email: email, Actor: "register", CodeHash: security.EmailCodeHash(a.Config.ClientSecretEncryptionKey, id.String(), in.EmailCode)},
		PasswordHash:   hash,
	})
	if err != nil {
		a.emailError(w, err)
		return
	}
	if !a.allowMaintenanceLogin(w, r, u) {
		return
	}
	if err := a.issueSession(w, r, u); err != nil {
		a.Log.Error("create registration session", "error", err)
		writeJSON(w, 500, map[string]string{"error": "server_error"})
		return
	}
	w.Header().Set("Location", "/api/v1/user")
	writeJSON(w, 201, userView(u))
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if !a.verifyHuman(w, r) {
		return
	}
	var in userInput
	if err := decodeJSON(r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	email := normalizeEmail(in.Email)
	limits := a.loginLimits(r, email)
	if !a.loginAllowed(w, r, limits) {
		return
	}
	u, err := a.Store.UserByEmail(r.Context(), email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.loginUncounted(r.Context(), limits)
		a.Log.Error("load user for login", "error", err)
		writeJSON(w, 500, map[string]string{"error": "server_error"})
		return
	}
	// A rejected attempt stays counted: loginAllowed reserved it up front.
	if err != nil || !u.IsActive {
		// Spend the same hashing time as a real check so response timing does
		// not reveal whether the email belongs to an active account.
		security.CheckPassword(a.decoyPasswordHash(), in.Password)
		writeJSON(w, 401, map[string]any{"error": "invalid_credentials"})
		return
	}
	if !security.CheckPassword(u.PasswordHash, in.Password) {
		writeJSON(w, 401, map[string]any{"error": "invalid_credentials"})
		return
	}
	a.loginSucceeded(r, email, limits)
	if !a.allowMaintenanceLogin(w, r, u) {
		return
	}
	if err := a.issueSession(w, r, u); err != nil {
		a.Log.Error("create session", "error", err)
		writeJSON(w, 500, map[string]string{"error": "server_error"})
		return
	}
	writeJSON(w, 200, userView(u))
}

// decoyPasswordHash is a hash at the configured cost that matches no password
// a client can know; it is derived once from a fresh random value.
func (a *API) decoyPasswordHash() string {
	a.decoyOnce.Do(func() {
		if value, err := security.RandomToken(32); err == nil {
			a.decoyHash, _ = security.HashPassword(value, a.Config.BCryptCost)
		}
	})
	return a.decoyHash
}

func (a *API) issueSession(w http.ResponseWriter, r *http.Request, user store.User) error {
	value, err := security.RandomToken(32)
	if err != nil {
		return err
	}
	expires := time.Now().Add(a.Config.SessionTTL)
	if err := a.Store.CreateSession(store.WithLoginDevice(r.Context(), a.loginDevice(r)), user.ID, security.HashToken(value), expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: a.Config.SessionCookieName, Value: value, Path: "/", Expires: expires, MaxAge: int(a.Config.SessionTTL.Seconds()), HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode})
	return nil
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(a.Config.SessionCookieName); err == nil {
		if err := a.Store.RevokeSession(r.Context(), security.HashToken(cookie.Value)); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "session_revocation_unavailable"})
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: a.Config.SessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) authorize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	in := oauth.AuthorizeRequest{ClientID: r.URL.Query().Get("client_id"), RedirectURI: r.URL.Query().Get("redirect_uri"), ResponseType: r.URL.Query().Get("response_type"), Scope: r.URL.Query().Get("scope"), State: r.URL.Query().Get("state"), CodeChallenge: r.URL.Query().Get("code_challenge"), CodeChallengeMethod: r.URL.Query().Get("code_challenge_method")}
	if err := validateState(in.State); err != nil {
		writeOAuthError(w, err)
		return
	}
	if _, err := a.oauth.ValidateAuthorize(r.Context(), in); err != nil {
		writeOAuthError(w, err)
		return
	}
	u, ok := a.CurrentUser(r)
	if !ok {
		loginURL := a.Config.AuthUIURL + "/login?return_to=" + url.QueryEscape(r.URL.RequestURI())
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}
	if u.ProfileSetupRequired {
		http.Redirect(w, r, a.Config.AuthUIURL+"/welcome?return_to="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	client, err := a.oauth.ValidateConsent(r.Context(), u.ID, in)
	if err != nil {
		writeOAuthError(w, err)
		return
	}
	a.beginConsent(w, r, u, client, in)
}

func (a *API) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, oauthErr("invalid_request", "invalid form body", 400))
		return
	}
	id, secret := clientCredentials(r)
	grant := r.PostForm.Get("grant_type")
	var response oauth.TokenResponse
	var oauthErrValue *oauth.Error
	switch grant {
	case "authorization_code":
		response, oauthErrValue = a.oauth.ExchangeCode(r.Context(), id, secret, r.PostForm.Get("code"), r.PostForm.Get("redirect_uri"), r.PostForm.Get("code_verifier"))
	case "refresh_token":
		response, oauthErrValue = a.oauth.Refresh(r.Context(), id, secret, r.PostForm.Get("refresh_token"), r.PostForm.Get("scope"))
	case "client_credentials":
		response, oauthErrValue = a.oauth.ClientCredentials(r.Context(), id, secret, r.PostForm.Get("scope"))
	default:
		oauthErrValue = oauthErr("unsupported_grant_type", "grant_type is not supported", 400)
	}
	if oauthErrValue != nil {
		writeOAuthError(w, oauthErrValue)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, 200, response)
}

func (a *API) revoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, oauthErr("invalid_request", "invalid form body", 400))
		return
	}
	id, secret := clientCredentials(r)
	if err := a.oauth.Revoke(r.Context(), id, secret, r.PostForm.Get("token")); err != nil {
		writeOAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
func (a *API) introspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, oauthErr("invalid_request", "invalid form body", 400))
		return
	}
	id, secret := clientCredentials(r)
	result, err := a.oauth.Introspect(r.Context(), id, secret, r.PostForm.Get("token"))
	if err != nil {
		writeOAuthError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var user store.User
	var scope string
	var tokenScopes []string
	usingToken := false
	if accessToken := bearer(r); accessToken != "" {
		usingToken = true
		token, err := a.oauth.AuthenticateAccessToken(r.Context(), accessToken)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeOAuthError(w, err)
			return
		}
		if token.UserID == nil {
			writeJSON(w, 403, map[string]string{"error": "insufficient_scope"})
			return
		}
		var dbErr error
		user, dbErr = a.Store.UserByID(r.Context(), *token.UserID)
		if dbErr != nil || !user.IsActive {
			writeJSON(w, 401, map[string]string{"error": "invalid_token"})
			return
		}
		scope = strings.Join(token.Scopes, " ")
		tokenScopes = token.Scopes
	} else {
		var ok bool
		user, ok = a.CurrentUser(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
			return
		}
	}
	view := userView(user)
	if usingToken {
		view = consentUserInfo(user, tokenScopes)
	}
	if scope != "" {
		view["scope"] = scope
	}
	writeJSON(w, 200, view)
}

func (a *API) passkeyRegistrationOptions(w http.ResponseWriter, r *http.Request) {
	user, ok := a.CurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
		return
	}
	options, ceremonyToken, err := a.passkey.BeginRegistration(r.Context(), user)
	if err != nil {
		a.Log.Error("begin passkey registration", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "passkey_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ceremony_token": ceremonyToken, "publicKey": options})
}

// A WebAuthn response is a few kilobytes; the library reads the body unbounded.
const maxPasskeyBody = 64 << 10

func (a *API) passkeyRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPasskeyBody)
	user, ok := a.CurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-WebAuthn-Ceremony"))
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ceremony_token_required"})
		return
	}
	name := strings.TrimSpace(r.Header.Get("X-Passkey-Name"))
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	if len(name) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "passkey_name_too_long"})
		return
	}
	credential, err := a.passkey.FinishRegistration(r.Context(), user, token, name, r)
	if errors.Is(err, store.ErrAlreadyExists) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "passkey_exists"})
		return
	}
	if err != nil {
		a.Log.Info("passkey registration rejected", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_passkey_registration"})
		return
	}
	writeJSON(w, http.StatusCreated, passkeyView(credential))
}

func (a *API) passkeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	if !a.verifyHuman(w, r) {
		return
	}
	options, ceremonyToken, err := a.passkey.BeginLogin(r.Context())
	if err != nil {
		a.Log.Error("begin passkey login", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "passkey_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ceremony_token": ceremonyToken, "publicKey": options})
}

func (a *API) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPasskeyBody)
	token := strings.TrimSpace(r.Header.Get("X-WebAuthn-Ceremony"))
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ceremony_token_required"})
		return
	}
	user, credential, err := a.passkey.FinishLogin(r.Context(), token, r)
	if err != nil {
		a.Log.Info("passkey login rejected", "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_passkey"})
		return
	}
	if !a.allowMaintenanceLogin(w, r, user) {
		return
	}
	if err := a.issueSession(w, r, user); err != nil {
		a.Log.Error("create passkey login session", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	view := userView(user)
	view["passkey_id"] = credential.ID
	writeJSON(w, http.StatusOK, view)
}

func (a *API) listPasskeys(w http.ResponseWriter, r *http.Request) {
	user, ok := a.CurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
		return
	}
	credentials, err := a.passkey.List(r.Context(), user.ID)
	if err != nil {
		a.Log.Error("list passkeys", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	views := make([]map[string]any, 0, len(credentials))
	for _, credential := range credentials {
		views = append(views, passkeyView(credential))
	}
	writeJSON(w, http.StatusOK, map[string]any{"passkeys": views})
}

func (a *API) deletePasskey(w http.ResponseWriter, r *http.Request) {
	user, ok := a.CurrentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_passkey_id"})
		return
	}
	if err := a.passkey.Delete(r.Context(), user.ID, id); errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "passkey_not_found"})
		return
	} else if err != nil {
		a.Log.Error("delete passkey", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func passkeyView(credential store.PasskeyCredential) map[string]any {
	return map[string]any{"id": credential.ID, "name": credential.Name, "created_at": credential.CreatedAt, "last_used_at": credential.LastUsedAt, "backup_eligible": credential.BackupEligible, "backup_state": credential.BackupState}
}

func userView(user store.User) map[string]any {
	return map[string]any{"id": user.ID, "bio": user.Bio, "background_url": user.BackgroundURL, "background_original_url": user.BackgroundOriginalURL, "avatar_original_url": user.AvatarOriginalURL, "numeric_id": user.NumericID, "email": user.Email, "email_verified": user.EmailVerified, "profile_setup_required": user.ProfileSetupRequired, "display_name": user.DisplayName, "role": user.Role, "level": store.RoleLevel(user.Role), "created_at": user.CreatedAt, "has_password": user.PasswordHash != "", "email_is_alias": strings.HasSuffix(user.Email, "@accounts.albatextus.invalid"), "avatar_source": user.AvatarSource, "avatar_url": user.AvatarURL, "picture": user.AvatarURL, "custom_avatar_url": user.CustomAvatarURL}
}

func clientCredentials(r *http.Request) (string, string) {
	if id, secret, ok := r.BasicAuth(); ok {
		return id, secret
	}
	return r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
}
func bearer(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if len(value) > 7 && strings.EqualFold(value[:7], "Bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}
func validateState(state string) *oauth.Error {
	if len(state) > 1024 {
		return oauthErr("invalid_request", "state is too long", 400)
	}
	return nil
}
func validateUserInput(in userInput) (string, string, *string, bool) {
	email := normalizeEmail(in.Email)
	if !validEmail(email) || len(in.Password) < 12 || len(in.Password) > 72 {
		return "", "", nil, false
	}
	var display *string
	if value := strings.TrimSpace(in.DisplayName); value != "" {
		if len(value) > 100 {
			return "", "", nil, false
		}
		display = &value
	}
	return email, in.Password, display, true
}
func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
func oauthErr(code, description string, status int) *oauth.Error {
	return &oauth.Error{Code: code, Description: description, Status: status}
}

func decodeJSON(r *http.Request, target any) error {
	if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return errors.New("content type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeOAuthError(w http.ResponseWriter, err *oauth.Error) {
	status := err.Status
	if status == 0 {
		status = 400
	}
	if err.Code == "invalid_client" {
		w.Header().Set("WWW-Authenticate", `Basic realm="oauth"`)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, err)
}
