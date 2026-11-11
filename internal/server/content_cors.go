package server

import (
	"net/http"
	"net/url"
	"strings"
)

// Content CORS permits browser reads from the configured account UI origins.
// Bearer authentication and request accounting still run on every GET.
func contentCORS(next http.Handler, origins []string) http.Handler {
	allowed := make(map[string]bool)
	for _, origin := range origins {
		u, err := url.Parse(origin)
		if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil {
			allowed[u.Scheme+"://"+u.Host] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2" && !strings.HasPrefix(r.URL.Path, "/api/v2/") &&
			r.URL.Path != "/api/v3/history" && r.URL.Path != "/api/v3/contributors" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			if r.Method == http.MethodOptions {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				if r.Header.Get("Access-Control-Request-Method") != http.MethodGet {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
					if header = strings.TrimSpace(header); header != "" && !strings.EqualFold(header, "authorization") {
						w.WriteHeader(http.StatusForbidden)
						return
					}
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if r.Method == http.MethodGet {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}
		next.ServeHTTP(w, r)
	})
}
