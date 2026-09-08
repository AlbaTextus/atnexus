package config

import (
	"fmt"
	"net/url"
	"strings"
)

func validateImgbed(c Config) error {
	if c.ImgbedURL == "" && c.ImgbedAPIToken == "" {
		return nil
	}
	u, err := url.Parse(c.ImgbedURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || c.ImgbedAPIToken == "" || strings.ContainsAny(c.ImgbedAPIToken, "\r\n") {
		return fmt.Errorf("IMGBED_URL must be an origin and IMGBED_API_TOKEN is required")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(c.Env != "production" && u.Scheme == "http" && local) {
		return fmt.Errorf("IMGBED_URL requires HTTPS (loopback HTTP is allowed in development)")
	}
	return nil
}
