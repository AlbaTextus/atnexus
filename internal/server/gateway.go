package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
)

// GatewayRoute mounts one module listener behind the aggregate port.
type GatewayRoute struct {
	Name string
	// Prefixes own the exact path and everything below it: "/api/v1" matches
	// "/api/v1" and "/api/v1/user" but not "/api/v10".
	Prefixes []string
	// Handler is the module listener's complete handler, so CORS, timeouts and
	// authentication behave exactly as they do on the module's own port.
	Handler http.Handler
	// Timeout is the module listener's read/write deadline.
	Timeout time.Duration
}

// NewGateway serves every module on one listener, dispatching by path prefix
// without changing the request. /healthz reports all modules together, using
// the same 200 / 433 / other convention as each module's own check.
func NewGateway(routes ...GatewayRoute) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			gatewayHealth(w, r, routes)
			return
		}
		for _, route := range routes {
			if !route.owns(r.URL.Path) {
				continue
			}
			// The aggregate listener allows the longest module deadline; narrow
			// it to the deadline this module's own listener would enforce.
			if route.Timeout > 0 {
				deadline := time.Now().Add(route.Timeout)
				controller := http.NewResponseController(w)
				_ = controller.SetReadDeadline(deadline)
				_ = controller.SetWriteDeadline(deadline)
			}
			route.Handler.ServeHTTP(w, r)
			return
		}
		apibase.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	})
}

func (route GatewayRoute) owns(path string) bool {
	for _, prefix := range route.Prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// statusMaintenance is the platform's planned-maintenance health status; the
// status monitor reports it as maintenance instead of an outage.
const statusMaintenance = 433

func gatewayHealth(w http.ResponseWriter, r *http.Request, routes []GatewayRoute) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		apibase.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	// One deadline for all probes: a stalled module cannot hold the check open.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	// A fresh request keeps the caller's Origin and credentials out of the probes.
	probe, err := http.NewRequestWithContext(ctx, http.MethodGet, "/healthz", nil)
	if err != nil {
		apibase.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_server_error"})
		return
	}
	code := http.StatusOK
	modules := make(map[string]string, len(routes))
	for _, route := range routes {
		result := &statusWriter{header: make(http.Header)}
		route.Handler.ServeHTTP(result, probe.Clone(ctx))
		switch result.status {
		case http.StatusOK:
			modules[route.Name] = "ok"
		case statusMaintenance:
			modules[route.Name] = "maintenance"
			// An outage elsewhere outranks planned maintenance.
			if code == http.StatusOK {
				code = statusMaintenance
				if retry := result.header.Get("Retry-After"); retry != "" {
					w.Header().Set("Retry-After", retry)
				}
			}
		default:
			modules[route.Name] = "unhealthy"
			code = http.StatusServiceUnavailable
		}
	}
	status := "ok"
	switch code {
	case statusMaintenance:
		status = "maintenance"
	case http.StatusServiceUnavailable:
		status = "unhealthy"
		w.Header().Del("Retry-After")
	}
	w.Header().Set("Cache-Control", "no-store")
	apibase.WriteJSON(w, code, map[string]any{"status": status, "modules": modules})
}

// statusWriter keeps only the status code of a module health probe.
type statusWriter struct {
	header http.Header
	status int
}

func (w *statusWriter) Header() http.Header { return w.header }
func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *statusWriter) Write(body []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return len(body), nil
}
