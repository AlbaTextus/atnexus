package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/google/uuid"
)

// ModuleHTTP applies the same browser/session transport rules to every listener.
// OPTIONS is handled before business authentication and accounting.
func ModuleHTTP(next http.Handler, timeout time.Duration, logger *slog.Logger, origins ...string) http.Handler {
	allowed := make(map[string]bool)
	for _, origin := range origins {
		u, err := url.Parse(origin)
		if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil {
			allowed[u.Scheme+"://"+u.Host] = true
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	// The timeout handler runs next on another goroutine and re-raises its
	// panic here, so the stack has to be taken where the panic happened.
	next = tracePanics(next)
	if timeout > 0 {
		next = http.TimeoutHandler(next, timeout, `{"error":"server_timeout"}`)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", uuid.NewString())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" && !allowed[origin] {
			apibase.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "invalid_origin"})
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			switch r.Header.Get("Access-Control-Request-Method") {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				apibase.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "invalid_preflight"})
				return
			}
			for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				switch strings.ToLower(strings.TrimSpace(header)) {
				case "", "authorization", "content-type", "x-albatextus-client":
				default:
					apibase.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "invalid_preflight"})
					return
				}
			}
			if origin == "" {
				apibase.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "invalid_origin"})
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-AlbaTextus-Client")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		defer func() {
			if value := recover(); value != nil {
				// net/http uses this sentinel to abort a response on purpose.
				if value == http.ErrAbortHandler {
					panic(value)
				}
				stack := debug.Stack()
				if traced, ok := value.(*tracedPanic); ok {
					value, stack = traced.value, traced.stack
				}
				logger.Error("module request panicked", "panic", fmt.Sprint(value), "method", r.Method, "path", r.URL.Path, "request_id", w.Header().Get("X-Request-ID"), "stack", string(stack))
				apibase.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_server_error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// tracedPanic carries a panic together with the stack of the goroutine it
// happened on.
type tracedPanic struct {
	value any
	stack []byte
}

func tracePanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				if value == http.ErrAbortHandler {
					panic(value)
				}
				panic(&tracedPanic{value: value, stack: debug.Stack()})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
