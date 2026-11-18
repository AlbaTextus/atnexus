package server

import (
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/AlbaTextus/atnexus/internal/apibase"
)

// NewATHubHandler exposes the v5 module boundary while the existing content
// engine, access policies and persisted audit labels retain their v2 contract.
func NewATHubHandler(content http.Handler, ping func(context.Context) error) http.Handler {
	legacy := NewContentHandler(content, ping)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health is answered before the content engine, so probes are neither
		// authenticated nor recorded as API calls.
		if r.URL.Path == "/api/v5/healthz" {
			request := r.Clone(r.Context())
			request.URL.Path, request.URL.RawPath = "/healthz", ""
			request.RequestURI = request.URL.RequestURI()
			legacy.ServeHTTP(w, request)
			return
		}
		if r.URL.Path != "/api/v5" && !strings.HasPrefix(r.URL.Path, "/api/v5/") {
			apibase.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		contentPath := "/api/v2" + strings.TrimPrefix(r.URL.Path, "/api/v5")
		// Illustration operations are public only through CreatPlat. Its internal
		// calls still use the existing content engine and policy labels.
		switch path.Clean(contentPath) {
		case "/api/v2/illustrations", "/api/v2/illustration-layout":
			apibase.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		request := r.Clone(r.Context())
		request.URL.Path = contentPath
		request.URL.RawPath = ""
		request.RequestURI = request.URL.RequestURI()
		legacy.ServeHTTP(&contentLocationWriter{ResponseWriter: w}, request)
	})
}

type contentLocationWriter struct{ http.ResponseWriter }

func (w *contentLocationWriter) rewriteLocation() {
	location := w.Header().Get("Location")
	if strings.HasPrefix(location, "/api/v2/") {
		w.Header().Set("Location", "/api/v5/"+strings.TrimPrefix(location, "/api/v2/"))
	}
}
func (w *contentLocationWriter) WriteHeader(status int) {
	w.rewriteLocation()
	w.ResponseWriter.WriteHeader(status)
}
func (w *contentLocationWriter) Write(body []byte) (int, error) {
	w.rewriteLocation()
	return w.ResponseWriter.Write(body)
}
func (w *contentLocationWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
