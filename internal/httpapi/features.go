package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
)

func (a *API) siteFeatures(ctx context.Context) (store.SiteFeatures, error) {
	st, ok := a.store.(store.SiteFeaturesStore)
	if !ok {
		// Lightweight test and alternate stores keep the historical behavior
		// until they opt into persistent feature settings.
		return store.SiteFeatures{ExternalLoginEnabled: true, RegistrationEnabled: true, TurnstileEnabled: true}, nil
	}
	return st.SiteFeatures(ctx)
}

func siteFeaturesUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{
		"error":             "site_features_unavailable",
		"error_description": "暂时无法读取站点功能设置，请稍后重试。",
	})
}

func (a *API) requireRegistration(w http.ResponseWriter, r *http.Request) bool {
	features, err := a.siteFeatures(r.Context())
	if err != nil {
		a.log.Error("read site features", "error", err)
		siteFeaturesUnavailable(w)
		return false
	}
	if !features.RegistrationEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error":             "registration_disabled",
			"error_description": "全站注册已关闭，已有账户仍可正常登录。",
		})
		return false
	}
	return true
}

func (a *API) featureSettingsView(features store.SiteFeatures) map[string]any {
	providerConfigured := false
	providers := make([]map[string]any, 0)
	for _, provider := range a.federation.Providers() {
		configured := provider.Enabled()
		providerConfigured = providerConfigured || configured
		if provider.ID == "custom" && !configured {
			continue
		}
		providers = append(providers, map[string]any{
			"id": provider.ID, "name": provider.Name,
			"enabled": features.LoginProviderEnabled(provider.ID), "configured": configured,
		})
	}
	return map[string]any{
		"external_login_enabled": features.ExternalLoginEnabled,
		"registration_enabled":   features.RegistrationEnabled,
		"turnstile_enabled":      features.TurnstileEnabled,
		"provider_configured":    providerConfigured,
		"providers":              providers,
		"turnstile_configured":   a.cfg.Turnstile.Enabled(),
		"updated_at":             features.UpdatedAt,
	}
}

func (a *API) featureSettingsActor(w http.ResponseWriter, r *http.Request) (store.User, store.SiteFeaturesStore, bool) {
	user, ok := a.accountActor(w, r)
	if !ok {
		return user, nil, false
	}
	if !isAdmin(user) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error":             "admin_required",
			"error_description": "站点功能设置仅限管理员使用。",
		})
		return user, nil, false
	}
	st, ok := a.store.(store.SiteFeaturesStore)
	if !ok {
		siteFeaturesUnavailable(w)
		return user, nil, false
	}
	return user, st, true
}

func (a *API) getSiteFeatures(w http.ResponseWriter, r *http.Request) {
	_, st, ok := a.featureSettingsActor(w, r)
	if !ok {
		return
	}
	features, err := st.SiteFeatures(r.Context())
	if err != nil {
		a.log.Error("read site features", "error", err)
		siteFeaturesUnavailable(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, a.featureSettingsView(features))
}

func (a *API) setSiteFeature(w http.ResponseWriter, r *http.Request) {
	user, st, ok := a.featureSettingsActor(w, r)
	if !ok {
		return
	}
	var input struct {
		Feature  string `json:"feature"`
		Provider string `json:"provider"`
		Enabled  *bool  `json:"enabled"`
	}
	if !readAccountJSON(w, r, &input) {
		return
	}
	if input.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled_required"})
		return
	}
	if input.Feature != "" && input.Provider != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_feature_target", "error_description": "每次只能修改一个功能开关或登录平台。"})
		return
	}
	var features store.SiteFeatures
	var err error
	if input.Provider != "" {
		features, err = st.SetLoginProvider(r.Context(), input.Provider, *input.Enabled, user.ID)
	} else {
		features, err = st.SetSiteFeature(r.Context(), input.Feature, *input.Enabled, user.ID)
	}
	if errors.Is(err, store.ErrNotFound) {
		if input.Provider != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_provider", "error_description": "未知的第三方登录平台。"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_feature", "error_description": "未知的功能开关。"})
		return
	}
	if err != nil {
		a.log.Error("update site feature", "feature", input.Feature, "provider", input.Provider, "error", err)
		siteFeaturesUnavailable(w)
		return
	}
	a.log.Info("site feature updated", "feature", input.Feature, "provider", input.Provider, "enabled", *input.Enabled, "actor", user.ID)
	writeJSON(w, http.StatusOK, a.featureSettingsView(features))
}
