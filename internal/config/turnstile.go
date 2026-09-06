package config

import (
	"errors"
	"net/url"
	"strings"
)

const TurnstileTestSiteKey = "1x00000000000000000000AA"
const TurnstileTestSecretKey = "1x0000000000000000000000000000000AA"

type Turnstile struct {
	SiteKey          string
	SecretKey        string
	TestMode         bool
	AllowedHostnames []string
}

func (c Turnstile) Enabled() bool { return c.SiteKey != "" && c.SecretKey != "" }

func (c Turnstile) Validate(environment string) error {
	if (c.SiteKey == "") != (c.SecretKey == "") {
		return errors.New("Turnstile requires both TURNSTILE_SITE_KEY and TURNSTILE_SECRET_KEY")
	}
	testSite := c.SiteKey == TurnstileTestSiteKey || c.SiteKey == "2x00000000000000000000AB" || c.SiteKey == "1x00000000000000000000BB" || c.SiteKey == "2x00000000000000000000BB" || c.SiteKey == "3x00000000000000000000FF"
	testSecret := c.SecretKey == TurnstileTestSecretKey || c.SecretKey == "2x0000000000000000000000000000000AA" || c.SecretKey == "3x0000000000000000000000000000000AA"
	if c.TestMode {
		if strings.EqualFold(environment, "production") || !testSite || !testSecret {
			return errors.New("Turnstile test mode requires official test keys and a non-production environment")
		}
	} else if testSite || testSecret {
		return errors.New("Turnstile test keys require TURNSTILE_TEST_MODE=true")
	}
	if c.Enabled() && !c.TestMode && len(c.AllowedHostnames) == 0 {
		return errors.New("Turnstile requires at least one allowed hostname")
	}
	for _, hostname := range c.AllowedHostnames {
		u, err := url.Parse("https://" + hostname)
		if err != nil || u.Hostname() == "" || u.Hostname() != hostname || u.Port() != "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(hostname, "*\\\r\n\t ") {
			return errors.New("TURNSTILE_ALLOWED_HOSTNAMES must contain exact hostnames without scheme, port, or path")
		}
	}
	return nil
}

func loadTurnstile(authUIURL string) (Turnstile, error) {
	testMode, err := getBool("TURNSTILE_TEST_MODE", false)
	if err != nil {
		return Turnstile{}, err
	}
	defaultHostname := ""
	if parsed, err := url.Parse(authUIURL); err == nil {
		defaultHostname = parsed.Hostname()
	}
	return Turnstile{SiteKey: getenv("TURNSTILE_SITE_KEY", ""), SecretKey: getenv("TURNSTILE_SECRET_KEY", ""), TestMode: testMode, AllowedHostnames: getList("TURNSTILE_ALLOWED_HOSTNAMES", defaultHostname)}, nil
}
