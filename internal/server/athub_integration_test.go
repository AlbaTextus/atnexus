package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"albatextus/athub"
	"github.com/AlbaTextus/oauth/internal/athubaccess"
	"github.com/AlbaTextus/oauth/internal/config"
	"github.com/AlbaTextus/oauth/internal/httpapi"
	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/server"
	"github.com/AlbaTextus/oauth/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func migratedSchema(t *testing.T, dsn, pattern string) string {
	t.Helper()
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "host_athub_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := pgx.Identifier{name}.Sanitize()
	if _, err := connection.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := connection.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		connection.Close(ctx)
	})
	if _, err := connection.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) == 0 {
		t.Fatalf("migrations not found: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := strings.Replace(string(data), `CREATE SCHEMA IF NOT EXISTS "public";`, "", 1)
		if _, err := connection.Exec(ctx, sql, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", name)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func TestATHubUserKeyContentFlowIntegration(t *testing.T) {
	accountDSN, contentDSN := os.Getenv("OAUTH_TEST_DATABASE_URL"), os.Getenv("ATHUB_TEST_DATABASE_URL")
	if accountDSN == "" || contentDSN == "" {
		t.Skip("set OAUTH_TEST_DATABASE_URL and ATHUB_TEST_DATABASE_URL")
	}
	accountDSN = migratedSchema(t, accountDSN, "../../prisma/migrations/*/migration.sql")
	contentDSN = migratedSchema(t, contentDSN, "../../modules/athub/prisma/migrations/*/migration.sql")
	ctx := context.Background()
	st, err := store.NewPostgres(ctx, accountDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	user, err := st.CreateUser(ctx, "collector@example.test", "unused", nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser(ctx, "other@example.test", "unused", nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, otherCookie := "test-session-"+uuid.NewString(), "test-session-"+uuid.NewString()
	if err := st.CreateSession(ctx, user.ID, security.HashToken(cookie), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession(ctx, other.ID, security.HashToken(otherCookie), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		Env: "development", SessionCookieName: "session", ClientSecretEncryptionKey: strings.Repeat("0", 64),
		WebAuthnRPID: "localhost", WebAuthnRPName: "ALBATEXTUS", WebAuthnRPOrigins: []string{"http://localhost:3288"},
		HTTPRequestTimeout: 30 * time.Second,
	}
	account, err := httpapi.New(cfg, st, logger)
	if err != nil {
		t.Fatal(err)
	}
	v1 := account.Router()
	access, err := athubaccess.New(st, ":8003", logger)
	if err != nil {
		t.Fatal(err)
	}
	content, err := athub.New(ctx, athub.Config{DatabaseURL: contentDSN, WrapHandler: access.Wrap}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	v2 := server.NewContentHandler(content.Handler(), content.Ping)
	call := func(handler http.Handler, method, path, body, session, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-AlbaTextus-Client", "account-center")
		if session != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: session})
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body)
		}
		return w
	}
	created := call(v1, "POST", "/api/v1/account/key", `{"name":"collector"}`, cookie, "", 201)
	var key struct {
		Token string         `json:"api_key"`
		Key   store.ATHubKey `json:"key"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	if key.Token == "" || key.Key.UserID != user.ID {
		t.Fatal("key not assigned to logged-in user")
	}
	call(v1, "POST", "/api/v1/account/key", `{"name":"spoof","user_id":"another"}`, cookie, "", 400)
	call(v1, "DELETE", "/api/v1/account/key/"+key.Key.ID.String(), "", otherCookie, "", 404)
	call(v2, "GET", "/healthz", "", "", "", 200)
	platformResponse := call(v2, "POST", "/api/v2/platforms", `{"name":"test"}`, "", key.Token, 201)
	var platform struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(platformResponse.Body.Bytes(), &platform); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"platformId": platform.ID, "title": "Audit integration novel"})
	novelResponse := call(v2, "POST", "/api/v2/novelinfo", string(body), "", key.Token, 201)
	var novel struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(novelResponse.Body.Bytes(), &novel); err != nil {
		t.Fatal(err)
	}
	query := url.Values{"novelId": []string{jsonNumber(novel.ID)}}
	call(v2, "GET", "/api/v2/content?"+query.Encode(), "", "", key.Token, 200)
	call(v2, "GET", "/api/v2/content?novelId=bad&token=must-not-be-recorded", "", "", key.Token, 400)
	call(v2, "GET", "/api/v2/athub/content", "", "", key.Token, 404)
	call(v1, "DELETE", "/api/v1/account/key/"+key.Key.ID.String(), "", cookie, "", 204)
	call(v2, "GET", "/api/v2/platforms", "", "", key.Token, 401)
	call(v2, "GET", "/api/v2/platforms", "", "", strings.Repeat("t", 64), 401)
	usageResponse := call(v1, "GET", "/api/v1/account/athub/usage", "", cookie, "", 200)
	var usage store.ATHubUsage
	if err := json.Unmarshal(usageResponse.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.Total != 6 || usage.Success != 3 || usage.Failed != 3 || usage.Pending != 0 {
		t.Fatalf("incorrect user accounting: %+v", usage)
	}
	otherUsageResponse := call(v1, "GET", "/api/v1/account/athub/usage", "", otherCookie, "", 200)
	if err := json.Unmarshal(otherUsageResponse.Body.Bytes(), &usage); err != nil || usage.Total != 0 {
		t.Fatal("another user's usage leaked")
	}
	callsResponse := call(v1, "GET", "/api/v1/account/athub/calls?limit=100", "", cookie, "", 200)
	if strings.Contains(callsResponse.Body.String(), key.Token) || strings.Contains(callsResponse.Body.String(), "must-not-be-recorded") {
		t.Fatal("audit contains secret data")
	}
	var recorded struct {
		Calls []store.ATHubCall `json:"calls"`
	}
	if err := json.Unmarshal(callsResponse.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	if len(recorded.Calls) != 6 {
		t.Fatal("missing call records")
	}
	for _, c := range recorded.Calls {
		if c.Port != 8003 || c.UserID == nil || *c.UserID != user.ID || c.Status == nil || c.CompletedAt == nil {
			t.Fatalf("incomplete call: %+v", c)
		}
	}
	list := call(v1, "GET", "/api/v1/account/key", "", cookie, "", 200)
	if strings.Contains(list.Body.String(), key.Token) || strings.Contains(list.Body.String(), "token_hash") {
		t.Fatal("key secret leaked in list")
	}
}

func jsonNumber(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}
