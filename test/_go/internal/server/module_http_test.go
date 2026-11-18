package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

func panicInTheHandler() { panic("handler broke") }

func TestModulePanicIsLoggedWithTheHandlerStack(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		var logged bytes.Buffer
		handler := ModuleHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panicInTheHandler() }), timeout, slog.New(slog.NewTextHandler(&logged, nil)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/users", nil))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("timeout %s: status = %d, want 500", timeout, w.Code)
		}
		if !strings.Contains(logged.String(), "handler broke") || !strings.Contains(logged.String(), "panicInTheHandler") {
			t.Fatalf("timeout %s: log does not point at the failing handler: %s", timeout, logged.String())
		}
	}
}

// moduleReply serves the module the way the server does: behind the gateway,
// whose connection deadline is the module's own request deadline.
func moduleReply(t *testing.T, method string, timeout time.Duration, handler http.HandlerFunc) (*http.Response, string) {
	t.Helper()
	server := httptest.NewServer(NewGateway(GatewayRoute{Prefixes: []string{"/api/v4"}, Handler: ModuleHTTP(handler, timeout, nil), Timeout: timeout}))
	defer server.Close()
	var sent io.Reader
	if method != http.MethodGet {
		sent = strings.NewReader(`{"novelId":2}`)
	}
	request, err := http.NewRequest(method, server.URL+"/api/v4/history", sent)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(body)
}

// Clients read the error code out of a JSON reply, and tell a JSON reply by its
// Content-Type. The timeout reply is written by the module wrapper itself, at
// the moment the listener would otherwise close the connection.
func TestModuleTimeoutReplyIsJSON(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response, body := moduleReply(t, method, 20*time.Millisecond, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/csv")
			_, _ = io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
		})
		if response.StatusCode != http.StatusServiceUnavailable || body != `{"error":"server_timeout"}` {
			t.Fatalf("%s: status=%d body=%s", method, response.StatusCode, body)
		}
		if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("%s: Content-Type = %q, want JSON", method, got)
		}
	}
}

func TestModuleTimeoutKeepsHandlerReplies(t *testing.T) {
	for _, tc := range []struct {
		name, contentType string
		status            int
		body, want        string
	}{
		{"typed body", "text/csv", 200, "a,b", "text/csv"},
		{"untyped body", "", 200, "plain words", "text/plain; charset=utf-8"},
		{"empty unavailable", "", 503, "", ""},
		{"typed unavailable", "text/plain; charset=utf-8", 503, `{"error":"server_timeout"}`, "text/plain; charset=utf-8"},
	} {
		response, body := moduleReply(t, http.MethodGet, time.Second, func(w http.ResponseWriter, r *http.Request) {
			if tc.contentType != "" {
				w.Header().Set("Content-Type", tc.contentType)
			}
			w.WriteHeader(tc.status)
			io.WriteString(w, tc.body)
		})
		if response.StatusCode != tc.status || body != tc.body || response.Header.Get("Content-Type") != tc.want {
			t.Errorf("%s: status=%d body=%q Content-Type=%q", tc.name, response.StatusCode, body, response.Header.Get("Content-Type"))
		}
	}
}

func TestATHubV5DoesNotExposeIllustrations(t *testing.T) {
	handler := NewATHubHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("removed endpoint reached content handler: %s %s", r.Method, r.URL.Path)
	}), nil)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v5/illustrations?novelId=7"},
		{"POST", "/api/v5/illustrations?novelId=7"},
		{"DELETE", "/api/v5/illustrations?novelId=7&id=4"},
		{"GET", "/api/v5/illustration-layout?novelId=7"},
		{"PUT", "/api/v5/illustration-layout?novelId=7"},
		{"POST", "/api/v5//illustrations/"},
		{"POST", "/api/v5/%69llustrations"},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"error":"not_found"`) {
			t.Errorf("%s %s: status=%d body=%s", tc.method, tc.path, w.Code, w.Body)
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
	for _, path := range []string{"/api/v2/novelinfo", "/api/v4/history", "/api/v3/contributors", "/healthz", "/api/v2/healthz"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("exposed wrong module %s", path)
		}
	}
}

// Health lives under the module prefix and never reaches the content engine,
// where it would be authenticated and recorded as an API call.
func TestATHubV5Health(t *testing.T) {
	healthy := true
	content := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("health probe reached the content engine: %s %s", r.Method, r.URL.Path)
	})
	handler := NewATHubHandler(content, func(context.Context) error {
		if !healthy {
			return errors.New("database unavailable")
		}
		return nil
	})
	for _, tc := range []struct {
		method  string
		healthy bool
		status  int
	}{{"GET", true, 200}, {"GET", false, 503}, {"POST", true, 405}} {
		healthy = tc.healthy
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, "/api/v5/healthz", nil))
		if w.Code != tc.status {
			t.Fatalf("%s healthy=%v status=%d, want=%d", tc.method, tc.healthy, w.Code, tc.status)
		}
	}
}
