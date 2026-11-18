package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
)

// GatewayRoute mounts one module listener behind the aggregate port.
type GatewayRoute struct {
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
// without changing the request. Health checks need no route of their own:
// each module answers /api/v{n}/healthz under its prefix.
func NewGateway(routes ...GatewayRoute) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, route := range routes {
			if !route.owns(r.URL.Path) {
				continue
			}
			// The aggregate listener allows the longest module deadline; narrow
			// it to the current module's deadline.
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
