package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func gatewayModule(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Module", name)
		w.Header().Set("Path", r.URL.RequestURI())
		w.Header().Set("Auth", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestGatewayDispatchesByPrefix(t *testing.T) {
	handler := NewGateway(
		GatewayRoute{Prefixes: []string{"/api/v1", "/oauth", "/.well-known"}, Handler: gatewayModule("oauth")},
		GatewayRoute{Prefixes: []string{"/api/v2", "/file"}, Handler: gatewayModule("image")},
		GatewayRoute{Prefixes: []string{"/api/v3"}, Handler: gatewayModule("mail")},
		GatewayRoute{Prefixes: []string{"/api/v4"}, Handler: gatewayModule("creatplat")},
		GatewayRoute{Prefixes: []string{"/api/v5"}, Handler: gatewayModule("athub")},
	)
	for _, tc := range []struct {
		path   string
		module string
	}{
		{"/api/v1/user", "oauth"}, {"/api/v1", "oauth"}, {"/oauth/token", "oauth"},
		{"/.well-known/oauth-authorization-server", "oauth"},
		{"/api/v2/manage/list?dir=book%2Fcovers", "image"}, {"/file/book/covers/a.webp", "image"},
		{"/api/v4/history?novelId=2", "creatplat"},
		{"/api/v5/content?novelId=7&order=1", "athub"}, {"/api/v5/", "athub"},
		{"/api/v1/healthz", "oauth"}, {"/api/v2/healthz", "image"},
		{"/api/v3/auth/email/code", "mail"}, {"/api/v3/healthz", "mail"},
		{"/api/v4/healthz", "creatplat"}, {"/api/v5/healthz", "athub"},
		{"/healthz", ""}, {"/api/healthz", ""}, {"/api/v6/healthz", ""},
		{"/api/v30/history", ""}, {"/api/v10/user", ""}, {"/api/v50", ""},
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

// Each module keeps its own deadline on the shared port.
func TestGatewayAppliesModuleDeadline(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	})
	gateway := httptest.NewServer(NewGateway(
		GatewayRoute{Prefixes: []string{"/api/v1"}, Handler: slow, Timeout: 50 * time.Millisecond},
		GatewayRoute{Prefixes: []string{"/api/v2"}, Handler: slow, Timeout: time.Minute},
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
