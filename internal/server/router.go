// Package server composes the independently maintained API modules.
package server

import (
	"context"
	"net/http"
	"time"
)

func NewContentHandler(v2 http.Handler, pingV2 func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	// Register both forms to avoid redirecting authenticated requests at the boundary.
	mux.Handle("/api/v2", v2)
	mux.Handle("/api/v2/", v2)
	mux.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if err := pingV2(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	return mux
}
