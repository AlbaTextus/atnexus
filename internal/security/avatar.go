package security

import (
	"net"
	"net/url"
	"strings"
)

// SafeAvatarURL accepts public HTTPS image references. Images are loaded by the
// browser; the account service never downloads provider-supplied URLs.
func SafeAvatarURL(value string) string {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || len(value) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(value, "\\\r\n\t ") {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".invalid") || !strings.Contains(host, ".") {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()) {
		return ""
	}
	return u.String()
}
