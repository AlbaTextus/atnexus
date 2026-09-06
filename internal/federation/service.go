package federation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/AlbaTextus/oauth/internal/config"
	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
	"golang.org/x/oauth2"
)

type Service struct {
	providers []config.LoginProvider
	baseURL   string
	client    *http.Client
}

func New(cfg config.Config) (*Service, error) {
	if err := config.ValidateLoginProviders(cfg.LoginProviders, cfg.Env); err != nil {
		return nil, err
	}
	return &Service{providers: cfg.LoginProviders, baseURL: strings.TrimRight(cfg.AuthUIURL, "/"), client: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("OAuth endpoint redirect refused") },
	}}, nil
}

func (s *Service) Providers() []config.LoginProvider { return s.providers }

func (s *Service) Provider(id string) (config.LoginProvider, bool) {
	for _, p := range s.providers {
		if p.ID == id && p.Enabled() {
			return p, true
		}
	}
	return config.LoginProvider{}, false
}

func (s *Service) oauthConfig(p config.LoginProvider) oauth2.Config {
	return oauth2.Config{ClientID: p.ClientID, ClientSecret: p.ClientSecret, RedirectURL: s.baseURL + "/api/v1/auth/providers/" + p.ID + "/callback", Scopes: p.Scopes,
		Endpoint: oauth2.Endpoint{AuthURL: p.AuthorizationURL, TokenURL: p.TokenURL, AuthStyle: oauth2.AuthStyleInParams}}
}

func (s *Service) AuthorizationURL(p config.LoginProvider, state, verifier string) string {
	c := s.oauthConfig(p)
	return c.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (s *Service) Exchange(ctx context.Context, p config.LoginProvider, code, verifier string) (store.ExternalIdentity, error) {
	c := s.oauthConfig(p)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.client)
	token, err := c.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil || token.AccessToken == "" {
		// Provider error bodies may contain credentials; never propagate them to logs or users.
		return store.ExternalIdentity{}, errors.New("OAuth code exchange failed")
	}
	var profile map[string]any
	if err := s.readProfile(ctx, p.UserInfoURL, token.AccessToken, &profile); err != nil {
		return store.ExternalIdentity{}, err
	}
	identity := store.ExternalIdentity{Provider: p.ID, Subject: stringField(profile, p.SubjectField), DisplayName: stringField(profile, p.NameField), Email: strings.ToLower(strings.TrimSpace(stringField(profile, p.EmailField)))}
	identity.EmailVerified, _ = field(profile, p.EmailVerifiedField).(bool)
	identity.AvatarURL = profileAvatarURL(p, profile)
	if p.ID == "github" {
		var emails []struct {
			Email    string `json:"email"`
			Verified bool   `json:"verified"`
			Primary  bool   `json:"primary"`
		}
		if err := s.readProfile(ctx, "https://api.github.com/user/emails", token.AccessToken, &emails); err != nil {
			return store.ExternalIdentity{}, err
		}
		for _, email := range emails {
			if email.Verified && (identity.Email == "" || email.Primary) {
				identity.Email = strings.ToLower(strings.TrimSpace(email.Email))
				identity.EmailVerified = true
				if email.Primary {
					break
				}
			}
		}
	}
	if identity.Subject == "" || len(identity.Subject) > 255 {
		if p.ID == "linuxdo" {
			identity.Subject = stringField(profile, "sub")
		}
	}
	if identity.Subject == "" || len(identity.Subject) > 255 {
		return store.ExternalIdentity{}, errors.New("OAuth profile has no stable account identifier")
	}
	if p.ID == "linuxdo" {
		if active, present := profile["active"].(bool); present && !active {
			return store.ExternalIdentity{}, errors.New("upstream account inactive")
		}
		if identity.DisplayName == "" {
			identity.DisplayName = stringField(profile, "name")
		}
		// LINUX DO does not advertise email_verified. Use a reserved local identifier,
		// never an unverified email that could collide with someone else's account.
		identity.Email = "linuxdo-" + security.HashToken(identity.Subject)[:32] + "@accounts.albatextus.invalid"
		identity.AccountAlias = true
	}
	parsed, err := mail.ParseAddress(identity.Email)
	if err != nil || parsed.Address != identity.Email || len(identity.Email) > 320 {
		identity.Email = ""
		identity.EmailVerified = false
	}
	name := []rune(strings.TrimSpace(identity.DisplayName))
	if len(name) > 100 {
		name = name[:100]
	}
	identity.DisplayName = string(name)
	return identity, nil
}

func (s *Service) readProfile(ctx context.Context, endpoint, accessToken string, target any) error {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("invalid OAuth profile endpoint")
	}
	r.Header.Set("Authorization", "Bearer "+accessToken)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("User-Agent", "AlbaTextus-OAuth")
	response, err := s.client.Do(r)
	if err != nil {
		return errors.New("OAuth profile request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("OAuth profile request rejected")
	}
	d := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	d.UseNumber()
	if err := d.Decode(target); err != nil {
		return errors.New("invalid OAuth profile response")
	}
	return nil
}

func field(profile map[string]any, path string) any {
	var value any = profile
	for _, part := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[part]
	}
	return value
}

func profileAvatarURL(p config.LoginProvider, profile map[string]any) string {
	avatarField := p.AvatarField
	if avatarField == "" {
		avatarField = "picture"
		if p.ID == "github" || p.ID == "linuxdo" {
			avatarField = "avatar_url"
		}
	}
	value := strings.TrimSpace(stringField(profile, avatarField))
	if p.ID == "linuxdo" {
		if value == "" {
			value = stringField(profile, "avatar_template")
		}
		if value == "" {
			value = stringField(profile, "picture")
		}
		value = strings.ReplaceAll(value, "{size}", "128")
		if strings.HasPrefix(value, "/") {
			base, _ := url.Parse("https://linux.do")
			if relative, err := url.Parse(value); err == nil {
				value = base.ResolveReference(relative).String()
			}
		}
	}
	return security.SafeAvatarURL(value)
}

func stringField(profile map[string]any, path string) string {
	switch value := field(profile, path).(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return ""
	}
}
