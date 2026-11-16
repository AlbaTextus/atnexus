package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModuleCORSPreflightAndOrigin(t *testing.T) {
	for _, scenario := range []struct {
		origin, method string
		want           int
		called         bool
	}{
		{"http://localhost:3288", "OPTIONS", 204, false},
		{"http://localhost:3288", "GET", 200, true},
		{"http://untrusted.test", "GET", 403, false},
	} {
		called := false
		handler := ModuleHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) }), time.Second, nil, "http://localhost:3288")
		r := httptest.NewRequest(scenario.method, "/api/v4/history", nil)
		r.Header.Set("Origin", scenario.origin)
		if scenario.method == "OPTIONS" {
			r.Header.Set("Access-Control-Request-Method", "POST")
			r.Header.Set("Access-Control-Request-Headers", "content-type,x-albatextus-client")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != scenario.want || called != scenario.called {
			t.Fatalf("status=%d called=%v", w.Code, called)
		}
		if scenario.want < 400 && (w.Header().Get("Access-Control-Allow-Origin") != scenario.origin || w.Header().Get("Access-Control-Allow-Credentials") != "true") {
			t.Fatal("missing credential CORS headers")
		}
	}
}

func TestATHubV5Boundary(t *testing.T) {
	seen := ""
	content := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.RequestURI()
		w.Header().Set("Location", "/api/v2/novelinfo?id=9")
		w.WriteHeader(201)
	})
	handler := NewATHubHandler(content, func(context.Context) error { return nil })
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/v5/novelinfo?view=page", nil))
	if w.Code != 201 || seen != "/api/v2/novelinfo?view=page" || w.Header().Get("Location") != "/api/v5/novelinfo?id=9" {
		t.Fatalf("status=%d path=%s location=%s", w.Code, seen, w.Header().Get("Location"))
	}
	for _, path := range []string{"/api/v2/novelinfo", "/api/v4/history", "/api/v3/contributors"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("exposed wrong module %s", path)
		}
	}
}
