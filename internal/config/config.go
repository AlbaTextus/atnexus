package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlbaTextus/oauth/internal/security"
)

type Config struct {
	Env                       string
	HTTPAddr                  string
	DatabaseURL               string
	IssuerURL                 string
	AuthUIURL                 string
	CookieSecure              bool
	SessionTTL                time.Duration
	AuthorizationCodeTTL      time.Duration
	HTTPRequestTimeout        time.Duration
	HTTPShutdownTimeout       time.Duration
	BCryptCost                int
	SessionCookieName         string
	WebAuthnRPID              string
	WebAuthnRPName            string
	WebAuthnRPOrigins         []string
	PasskeyChallengeTTL       time.Duration
	ClientSecretEncryptionKey string
	LoginProviders            []LoginProvider
	Turnstile                 Turnstile
	ImgbedURL                 string
	ImgbedAPIToken            string
	SMTP                      SMTP
}

func Load() (Config, error) {
	c := Config{
		Env:                       getenv("APP_ENV", "development"),
		HTTPAddr:                  getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		IssuerURL:                 strings.TrimRight(getenv("ISSUER_URL", "http://localhost:8080"), "/"),
		AuthUIURL:                 strings.TrimRight(getenv("AUTH_UI_URL", "http://localhost:3000"), "/"),
		SessionTTL:                getDuration("SESSION_TTL", 24*time.Hour),
		AuthorizationCodeTTL:      getDuration("AUTHORIZATION_CODE_TTL", 5*time.Minute),
		HTTPRequestTimeout:        getDuration("HTTP_REQUEST_TIMEOUT", 15*time.Second),
		HTTPShutdownTimeout:       getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		BCryptCost:                getInt("BCRYPT_COST", 12),
		SessionCookieName:         getenv("SESSION_COOKIE_NAME", "albatextus_session"),
		WebAuthnRPID:              getenv("WEBAUTHN_RP_ID", "localhost"),
		WebAuthnRPName:            getenv("WEBAUTHN_RP_NAME", "ALBATEXTUS"),
		WebAuthnRPOrigins:         getList("WEBAUTHN_RP_ORIGINS", "http://localhost:3000"),
		PasskeyChallengeTTL:       getDuration("PASSKEY_CHALLENGE_TTL", 5*time.Minute),
		ClientSecretEncryptionKey: strings.TrimSpace(os.Getenv("CLIENT_SECRET_ENCRYPTION_KEY")),
		LoginProviders:            loadLoginProviders(),
		ImgbedURL:                 strings.TrimRight(strings.TrimSpace(os.Getenv("IMGBED_URL")), "/"),
		ImgbedAPIToken:            strings.TrimSpace(os.Getenv("IMGBED_API_TOKEN")),
	}
	var err error
	if strings.EqualFold(c.WebAuthnRPName, "ALBATEXTUS") {
		c.WebAuthnRPName = "ALBATEXTUS"
	}
	c.SMTP, err = loadSMTP()
	if err != nil {
		return Config{}, err
	}
	if err := validateImgbed(c); err != nil {
		return Config{}, err
	}
	c.Turnstile, err = loadTurnstile(c.AuthUIURL)
	if err != nil {
		return Config{}, err
	}
	if err := c.Turnstile.Validate(c.Env); err != nil {
		return Config{}, err
	}
	if err := ValidateLoginProviders(c.LoginProviders, c.Env); err != nil {
		return Config{}, err
	}
	c.CookieSecure, err = getBool("COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if _, err := security.NewSecretCipher(c.ClientSecretEncryptionKey); err != nil {
		return Config{}, err
	}
	if c.BCryptCost < 10 || c.BCryptCost > 15 {
		return Config{}, fmt.Errorf("BCRYPT_COST must be between 10 and 15")
	}
	if c.SessionTTL <= 0 || c.AuthorizationCodeTTL <= 0 || c.PasskeyChallengeTTL <= 0 {
		return Config{}, errors.New("session, authorization code, and passkey challenge TTL must be positive")
	}
	return c, nil
}

func getList(key, fallback string) []string {
	value := getenv(key, fallback)
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return d
}

func getInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func getBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return b, nil
}
