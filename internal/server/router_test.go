package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentListenerRouting(t *testing.T) {
	v2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Path", r.URL.RequestURI())
		w.Header().Set("Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	})
	handler := NewContentHandler(v2, func(context.Context) error { return nil })
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/v2/content?novelId=7&order=1", 204},
		{"/api/v2/upload", 204}, {"/api/v2", 204}, {"/api/v2/", 204},
		{"/api/v1/user", 404}, {"/oauth/token", 404},
		{"/.well-known/oauth-authorization-server", 404},
		{"/api/v20/content", 404}, {"/api/content", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.path, nil)
			r.Header.Set("Authorization", "Bearer test")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d, want=%d", w.Code, tc.status)
			}
			if tc.status == 204 && (w.Header().Get("Path") != tc.path || w.Header().Get("Auth") != "Bearer test") {
				t.Fatal("v2 path, query or authorization changed during dispatch")
			}
		})
	}
}

func TestContentHealth(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		handler := NewContentHandler(http.NotFoundHandler(), func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("health probe must have a deadline")
			}
			if unavailable {
				return errors.New("database unavailable")
			}
			return nil
		})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
		want := 200
		if unavailable {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("status=%d, want=%d", w.Code, want)
		}
	}
}
