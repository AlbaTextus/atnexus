package httpapi

import (
	"errors"
	"net/http"

	"github.com/AlbaTextus/oauth/internal/turnstile"
)

func (a *API) authVerification(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	c := a.cfg.Turnstile
	features, err := a.siteFeatures(r.Context())
	if err != nil {
		a.log.Error("read site features for auth verification", "error", err)
		siteFeaturesUnavailable(w)
		return
	}
	writeJSON(w, 200, map[string]any{
		"enabled":                features.TurnstileEnabled && c.Enabled(),
		"site_key":               c.SiteKey,
		"test_mode":              c.TestMode,
		"action":                 turnstile.Action,
		"email_enabled":          a.cfg.SMTP.Enabled(),
		"registration_enabled":   features.RegistrationEnabled,
		"external_login_enabled": features.ExternalLoginEnabled,
	})
}

func (a *API) verifyHuman(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	features, err := a.siteFeatures(r.Context())
	if err != nil {
		a.log.Error("read site features for human verification", "error", err)
		siteFeaturesUnavailable(w)
		return false
	}
	if !features.TurnstileEnabled {
		return true
	}
	err = a.turnstile.Verify(r.Context(), r.Header.Get("X-Turnstile-Token"))
	if err == nil {
		return true
	}
	code, message, status := "turnstile_failed", "安全验证未通过或已过期，请重新验证。", http.StatusForbidden
	if errors.Is(err, turnstile.ErrRequired) {
		code, message = "turnstile_required", "请先完成安全验证。"
	} else if errors.Is(err, turnstile.ErrUnavailable) {
		code, message, status = "turnstile_unavailable", "安全验证服务暂时不可用，请稍后重试。", http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": message})
	return false
}
