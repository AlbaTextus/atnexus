package httpapi

import (
	"encoding/hex"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlbaTextus/connect/internal/store"
)

func (a *API) loginDevice(r *http.Request) store.LoginDevice {
	device := store.LoginDevice{IP: loginIP(r, a.cfg.TrustedProxyCIDRs), LoggedInAt: time.Now().UTC()}
	// Bound untrusted metadata before persistence.
	ua := []rune(strings.ToValidUTF8(r.UserAgent(), ""))
	if len(ua) > 512 {
		ua = ua[:512]
	}
	device.UserAgent = string(ua)
	if cookie, err := r.Cookie("albatextus_screen"); err == nil && len(cookie.Value) <= 11 {
		parts := strings.Split(cookie.Value, "x")
		if len(parts) == 2 {
			width, e1 := strconv.Atoi(parts[0])
			height, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil && width > 0 && height > 0 && width <= 32768 && height <= 32768 {
				device.Screen = strconv.Itoa(width) + "x" + strconv.Itoa(height)
			}
		}
	}
	if cookie, err := r.Cookie("albatextus_timezone"); err == nil && len(cookie.Value) <= 128 {
		if zone, err := url.QueryUnescape(cookie.Value); err == nil && len(zone) <= 64 && zone != "" {
			if _, err := time.LoadLocation(zone); err == nil {
				device.Timezone = zone
			}
		}
	}
	if cookie, err := r.Cookie("albatextus_device"); err == nil && len(cookie.Value) == 64 {
		if _, err := hex.DecodeString(cookie.Value); err == nil {
			device.Fingerprint = strings.ToLower(cookie.Value)
		}
	}
	return device
}

func loginIP(r *http.Request, trusted []string) string {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	var address netip.Addr
	if err == nil {
		address = peer.Addr().Unmap()
	} else {
		address, _ = netip.ParseAddr(r.RemoteAddr)
	}
	if !address.IsValid() {
		return ""
	}
	isTrusted := func(ip netip.Addr) bool {
		for _, cidr := range trusted {
			if prefix, err := netip.ParsePrefix(cidr); err == nil && prefix.Contains(ip.Unmap()) {
				return true
			}
		}
		return false
	}
	if isTrusted(address) {
		chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(chain) - 1; i >= 0 && isTrusted(address); i-- {
			next, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
			if err != nil {
				break
			}
			address = next.Unmap()
		}
	}
	return address.String()
}
