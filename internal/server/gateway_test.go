package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func gatewayModule(name string, healthy bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			if r.Header.Get("Origin") != "" || r.Header.Get("Authorization") != "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if !healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("Module", name)
		w.Header().Set("Path", r.URL.RequestURI())
		w.Header().Set("Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	})
}

func testGateway(athubHealthy bool) http.Handler {
	return NewGateway(
		GatewayRoute{Name: "oauth", Prefixes: []string{"/api/v1", "/oauth", "/.well-known"}, Handler: gatewayModule("oauth", true)},
		GatewayRoute{Name: "image", Prefixes: []string{"/api/v2", "/file"}, Handler: gatewayModule("image", true)},
		GatewayRoute{Name: "creatplat", Prefixes: []string{"/api/v4"}, Handler: gatewayModule("creatplat", true)},
		GatewayRoute{Name: "athub", Prefixes: []string{"/api/v5"}, Handler: gatewayModule("athub", athubHealthy)},
	)
}

func TestGatewayDispatchesByPrefix(t *testing.T) {
	handler := testGateway(true)
	for _, tc := range []struct {
		path   string
		module string
	}{
		{"/api/v1/user", "oauth"}, {"/api/v1", "oauth"}, {"/oauth/token", "oauth"},
		{"/.well-known/oauth-authorization-server", "oauth"},
		{"/api/v2/manage/list?dir=book%2Fcovers", "image"}, {"/file/book/covers/a.webp", "image"},
		{"/api/v4/history?novelId=2", "creatplat"},
		{"/api/v5/content?novelId=7&order=1", "athub"}, {"/api/v5/", "athub"},
		{"/api/v3/history", ""}, {"/api/v10/user", ""}, {"/api/v50", ""},
		{"/api", ""}, {"/", ""}, {"/files/a.webp", ""}, {"/oauthx", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest("POST", tc.path, nil)
			r.Header.Set("Authorization", "Bearer test")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if tc.module == "" {
				if w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != `{"error":"not_found"}` {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				return
			}
			if w.Code != http.StatusNoContent || w.Header().Get("Module") != tc.module {
				t.Fatalf("status=%d module=%q, want %q", w.Code, w.Header().Get("Module"), tc.module)
			}
			if w.Header().Get("Path") != tc.path || w.Header().Get("Auth") != "Bearer test" {
				t.Fatal("path, query or authorization changed during dispatch")
			}
		})
	}
}

func TestGatewayHealthReportsEveryModule(t *testing.T) {
	for _, tc := range []struct {
		name    string
		healthy bool
		status  int
		athub   string
	}{{"healthy", true, 200, "ok"}, {"one module down", false, 503, "unhealthy"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/healthz", nil)
			// Caller headers must not reach the module probes.
			r.Header.Set("Origin", "https://attacker.example")
			r.Header.Set("Authorization", "Bearer test")
			w := httptest.NewRecorder()
			testGateway(tc.healthy).ServeHTTP(w, r)
			var body struct {
				Status  string            `json:"status"`
				Modules map[string]string `json:"modules"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.Modules["athub"] != tc.athub || body.Modules["oauth"] != "ok" || len(body.Modules) != 4 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if (body.Status == "ok") != tc.healthy || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%q cache=%q", body.Status, w.Header().Get("Cache-Control"))
			}
		})
	}
	w := httptest.NewRecorder()
	testGateway(true).ServeHTTP(w, httptest.NewRequest("POST", "/healthz", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz status=%d", w.Code)
	}
}

// Each module keeps its own listener deadline behind the shared port.
func TestGatewayAppliesModuleDeadline(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	})
	gateway := httptest.NewServer(NewGateway(
		GatewayRoute{Name: "short", Prefixes: []string{"/api/v1"}, Handler: slow, Timeout: 50 * time.Millisecond},
		GatewayRoute{Name: "long", Prefixes: []string{"/api/v2"}, Handler: slow, Timeout: time.Minute},
	))
	defer gateway.Close()
	response, err := http.Get(gateway.URL + "/api/v2/upload")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "done" {
		t.Fatalf("long route body=%q", body)
	}
	response, err = http.Get(gateway.URL + "/api/v1/user")
	if err == nil {
		body, _ = io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) == "done" {
			t.Fatal("short route answered after its module deadline")
		}
	}
}
