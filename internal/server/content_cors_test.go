package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentCORS(t *testing.T) {
	for _, tc := range []struct {
		name, method, origin, requestedMethod, requestedHeaders string
		wantStatus                                              int
		allow, dispatch                                         bool
	}{
		{"preflight", "OPTIONS", "https://account.example", "GET", "authorization", 204, true, false},
		{"read", "GET", "https://account.example", "", "", 401, true, true},
		{"different origin", "GET", "https://evil.example", "", "", 401, false, true},
		{"different preflight", "OPTIONS", "https://evil.example", "GET", "authorization", 401, false, true},
		{"no origin", "GET", "", "", "", 401, false, true},
		{"write preflight", "OPTIONS", "https://account.example", "POST", "authorization", 403, false, false},
		{"other headers", "OPTIONS", "https://account.example", "GET", "authorization, x-extra", 403, false, false},
		{"prefix attack", "GET", "https://account.example.evil.test", "", "", 401, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dispatched := false
			handler := contentCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched = true
				w.WriteHeader(http.StatusUnauthorized)
			}), []string{"https://account.example/path"})
			req := httptest.NewRequest(tc.method, "/api/v2/novelinfo", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", tc.requestedMethod)
			req.Header.Set("Access-Control-Request-Headers", tc.requestedHeaders)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.wantStatus || dispatched != tc.dispatch {
				t.Fatalf("status=%d dispatched=%v", w.Code, dispatched)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); (got != "") != tc.allow || (tc.allow && got != tc.origin) {
				t.Fatalf("allow-origin=%q", got)
			}
			if w.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("content reads must not share browser cookies")
			}
		})
	}
}

func TestV3HistoryCORS(t *testing.T) {
	for _, path := range []string{"/api/v3/history", "/api/v3/contributors"} {
		handler := NewContentHandler(http.NotFoundHandler(), nil, "https://account.example")
		r := httptest.NewRequest("OPTIONS", path+"?novelId=2", nil)
		r.Header.Set("Origin", "https://account.example")
		r.Header.Set("Access-Control-Request-Method", "GET")
		r.Header.Set("Access-Control-Request-Headers", "authorization")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "https://account.example" {
			t.Fatalf("%s preflight failed: %d", path, w.Code)
		}
	}
}
