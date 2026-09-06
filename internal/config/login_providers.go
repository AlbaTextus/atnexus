package config

import (
	"fmt"
	"net/url"
	"strings"
)

// LoginProvider describes an upstream OAuth service; none of its credentials are public.
type LoginProvider struct {
	ID, Name, ClientID, ClientSecret                        string
	AuthorizationURL, TokenURL, UserInfoURL                 string
	Scopes                                                  []string
	SubjectField, EmailField, EmailVerifiedField, NameField string
	AvatarField                                             string
}

func (p LoginProvider) Enabled() bool { return p.ClientID != "" && p.ClientSecret != "" }

func loadLoginProviders() []LoginProvider {
	return []LoginProvider{
		{ID: "github", Name: "GitHub", ClientID: getenv("GITHUB_CLIENT_ID", ""), ClientSecret: getenv("GITHUB_CLIENT_SECRET", ""),
			AuthorizationURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", UserInfoURL: "https://api.github.com/user", Scopes: []string{"read:user", "user:email"}, SubjectField: "id", NameField: "login", AvatarField: "avatar_url"},
		{ID: "linuxdo", Name: "LINUX DO", ClientID: getenv("LINUXDO_CLIENT_ID", ""), ClientSecret: getenv("LINUXDO_CLIENT_SECRET", ""),
			AuthorizationURL: "https://connect.linux.do/oauth2/authorize", TokenURL: "https://connect.linux.do/oauth2/token", UserInfoURL: "https://connect.linux.do/api/user", Scopes: []string{"openid", "profile", "email"}, SubjectField: "id", NameField: "username", AvatarField: "avatar_url"},
		{ID: "custom", Name: getenv("CUSTOM_OAUTH_NAME", "其他账号"), ClientID: getenv("CUSTOM_OAUTH_CLIENT_ID", ""), ClientSecret: getenv("CUSTOM_OAUTH_CLIENT_SECRET", ""),
			AuthorizationURL: getenv("CUSTOM_OAUTH_AUTHORIZATION_URL", ""), TokenURL: getenv("CUSTOM_OAUTH_TOKEN_URL", ""), UserInfoURL: getenv("CUSTOM_OAUTH_USERINFO_URL", ""), Scopes: getList("CUSTOM_OAUTH_SCOPES", "openid,profile,email"),
			SubjectField: getenv("CUSTOM_OAUTH_SUBJECT_FIELD", "sub"), EmailField: getenv("CUSTOM_OAUTH_EMAIL_FIELD", "email"), EmailVerifiedField: getenv("CUSTOM_OAUTH_EMAIL_VERIFIED_FIELD", "email_verified"), NameField: getenv("CUSTOM_OAUTH_NAME_FIELD", "name"), AvatarField: getenv("CUSTOM_OAUTH_AVATAR_FIELD", "picture")},
	}
}

func ValidateLoginProviders(providers []LoginProvider, environment string) error {
	seen := map[string]bool{}
	for _, p := range providers {
		if (p.ID != "github" && p.ID != "linuxdo" && p.ID != "custom") || seen[p.ID] {
			return fmt.Errorf("invalid or duplicate OAuth login provider")
		}
		seen[p.ID] = true
		if len(p.Name) == 0 || len(p.Name) > 100 {
			return fmt.Errorf("%s provider name must contain 1-100 bytes", p.ID)
		}
		if (p.ClientID == "") != (p.ClientSecret == "") {
			return fmt.Errorf("%s requires both client ID and client secret", p.ID)
		}
		if !p.Enabled() {
			continue
		}
		for _, endpoint := range []string{p.AuthorizationURL, p.TokenURL, p.UserInfoURL} {
			if !validProviderEndpoint(endpoint, environment) {
				return fmt.Errorf("%s requires valid HTTPS OAuth endpoints (loopback HTTP is allowed in development)", p.ID)
			}
		}
		if strings.TrimSpace(p.SubjectField) == "" {
			return fmt.Errorf("%s requires a subject field", p.ID)
		}
	}
	return nil
}

func validProviderEndpoint(value, environment string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(value, "\\\r\n\t ") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return environment != "production" && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
