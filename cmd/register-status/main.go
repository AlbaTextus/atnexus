// Register the adjacent Status application without reseeding users or rotating secrets.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/AlbaTextus/connect/internal/security"
	"github.com/AlbaTextus/connect/internal/store"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

const clientID = "albatextus-status"
const applicationName = "ALBATEXTUS Status"

func main() {
	appURL := flag.String("app-url", "http://127.0.0.1:3100", "Status origin")
	issuerURL := flag.String("issuer-url", "", "Public Connect origin; defaults to ISSUER_URL")
	adminID := flag.String("admin-id", "", "Existing administrator UUID; defaults to the SEED_ADMIN_EMAIL account")
	output := flag.String("output", "../status/.dev.vars", "Local Status environment file")
	migrateOwnerAllowlist := flag.Bool("migrate-owner-allowlist", false, "Replace the legacy owner-only Status allowlist with administrator role access")
	flag.Parse()
	if err := run(*appURL, *issuerURL, *adminID, *output, *migrateOwnerAllowlist); err != nil {
		fmt.Fprintln(os.Stderr, "Status registration failed:", err)
		os.Exit(1)
	}
}

func origin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("application and issuer URLs must be origins without credentials, paths, queries or fragments")
	}
	loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return "", errors.New("HTTPS is required except for loopback development origins")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func run(appURL, issuerURL, adminID, output string, migrateOwnerAllowlist bool) error {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return errors.New("cannot read Connect environment")
	}
	if issuerURL == "" {
		issuerURL = os.Getenv("ISSUER_URL")
	}
	if issuerURL == "" {
		issuerURL = "http://localhost:3000"
	}
	var err error
	if appURL, err = origin(appURL); err != nil {
		return err
	}
	if issuerURL, err = origin(issuerURL); err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	previous, err := os.ReadFile(output)
	if err != nil && !os.IsNotExist(err) {
		return errors.New("cannot read Status environment")
	}
	values, err := godotenv.Unmarshal(string(previous))
	if err != nil {
		return errors.New("Status environment is not valid dotenv")
	}
	if id := values["STATUS_OAUTH_CLIENT_ID"]; id != "" && id != clientID {
		return errors.New("Status already uses a different OAuth client; use a separate output file to review the Connect configuration")
	}
	if issuer := values["STATUS_OAUTH_ISSUER"]; issuer != "" && strings.TrimRight(issuer, "/") != issuerURL {
		return errors.New("Status already uses a different issuer; use a separate output file to review the Connect configuration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := store.NewPostgres(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("cannot connect to the configured Connect database")
	}
	defer st.Close()
	cipher, err := security.NewSecretCipher(os.Getenv("CLIENT_SECRET_ENCRYPTION_KEY"))
	if err != nil {
		return err
	}
	var admin store.User
	if adminID != "" {
		id, parseErr := uuid.Parse(adminID)
		if parseErr != nil {
			return errors.New("admin-id must be a user UUID")
		}
		admin, err = st.UserByID(ctx, id)
	} else {
		email := strings.TrimSpace(os.Getenv("SEED_ADMIN_EMAIL"))
		if email == "" {
			email = "admin@example.com"
		}
		admin, err = st.UserByEmail(ctx, email)
	}
	if err != nil || !admin.IsActive || (admin.Role != "ADMIN" && admin.Role != "SUPER_ADMIN") {
		return errors.New("an active Connect administrator is required; select an existing account with -admin-id")
	}

	// Existing ID restrictions are preserved unless the legacy owner-only default
	// is explicitly migrated. Never silently widen a custom account allowlist.
	if migrateOwnerAllowlist {
		allowedIDs := strings.TrimSpace(values["STATUS_OAUTH_ALLOWED_USER_IDS"])
		if allowedIDs != "" && allowedIDs != admin.ID.String() {
			return errors.New("cannot migrate a custom Status account allowlist; review STATUS_OAUTH_ALLOWED_USER_IDS first")
		}
		delete(values, "STATUS_OAUTH_ALLOWED_USER_IDS")
	}
	if strings.TrimSpace(values["STATUS_OAUTH_ALLOWED_ROLES"]) == "" {
		values["STATUS_OAUTH_ALLOWED_ROLES"] = "ADMIN,SUPER_ADMIN"
	}
	values["STATUS_OAUTH_ROLE_CLAIM"] = "role"

	callback := appURL + "/api/auth/callback"
	app, err := st.ApplicationByID(ctx, clientID, admin.ID, true)
	var secret string
	if errors.Is(err, store.ErrNotFound) {
		secret, err = security.RandomToken(32)
		if err != nil {
			return err
		}
		hash, err := security.HashPassword(secret, 12)
		if err != nil {
			return err
		}
		encrypted, err := cipher.Encrypt(clientID, secret)
		if err != nil {
			return err
		}
		app, err = st.CreateApplication(ctx, store.Application{
			ID: clientID, Name: applicationName, OwnerID: &admin.ID,
			HomepageURL: appURL, Description: "服务状态监控与公告管理后台",
			IconURL: appURL + "/icon.svg", MinimumRole: "ADMIN", IsActive: true,
			RedirectURIs: []string{callback}, AllowedScopes: []string{"profile"},
			AllowedGrantTypes: []string{"authorization_code"},
		}, &hash, &encrypted)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if app.Name != applicationName || app.IsPublic || app.OwnerID == nil || *app.OwnerID != admin.ID {
			return errors.New("client ID already belongs to a different application or owner")
		}
		if !app.IsActive || !slices.Contains(app.AllowedScopes, "profile") || !slices.Contains(app.AllowedGrantTypes, "authorization_code") {
			return errors.New("existing Status client must be active and support profile and authorization_code")
		}
		encrypted, err := st.ApplicationSecret(ctx, clientID, admin.ID, true)
		if err != nil || encrypted == nil {
			return errors.New("existing Status client has no recoverable secret")
		}
		secret, err = cipher.Decrypt(clientID, *encrypted)
		if err != nil {
			return err
		}
		changed := false
		if !slices.Contains(app.RedirectURIs, callback) {
			if len(app.RedirectURIs) >= 10 {
				return errors.New("existing Status client already has ten redirect URIs")
			}
			app.RedirectURIs = append(app.RedirectURIs, callback)
			changed = true
		}
		if app.MinimumRole == "USER" {
			app.MinimumRole = "ADMIN"
			changed = true
		}
		if changed {
			if _, err = st.UpdateApplication(ctx, app, admin.ID, true); err != nil {
				return err
			}
		}
	}

	values["STATUS_OAUTH_NAME"] = "ALBATEXTUS Connect"
	values["STATUS_OAUTH_CLIENT_ID"] = clientID
	values["STATUS_OAUTH_CLIENT_SECRET"] = secret
	values["STATUS_OAUTH_ISSUER"] = issuerURL
	values["STATUS_OAUTH_AUTHORIZATION_URL"] = issuerURL + "/oauth/authorize"
	values["STATUS_OAUTH_TOKEN_URL"] = issuerURL + "/oauth/token"
	values["STATUS_OAUTH_USERINFO_URL"] = issuerURL + "/api/v1/user"
	values["STATUS_OAUTH_REDIRECT_URI"] = callback
	values["STATUS_OAUTH_SCOPES"] = "profile"
	values["STATUS_OAUTH_USER_ID_CLAIM"] = "sub"
	// Connect reads form credentials directly; this also preserves URL-sensitive characters.
	values["STATUS_OAUTH_TOKEN_AUTH_METHOD"] = "client_secret_post"
	contents, err := godotenv.Marshal(values)
	if err != nil {
		return err
	}
	contents += "\n"
	if bytes.Contains(previous, []byte("\r\n")) {
		contents = strings.ReplaceAll(contents, "\n", "\r\n")
	}
	tempDir := filepath.Join(filepath.Dir(output), "temp")
	if err = os.MkdirAll(tempDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(tempDir, "connect-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(contents); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	current, err := os.ReadFile(output)
	if (err != nil && !os.IsNotExist(err)) || !bytes.Equal(current, previous) {
		return errors.New("Status environment changed during registration; rerun to preserve the latest changes")
	}
	if err = os.Rename(f.Name(), output); err != nil {
		return err
	}
	fmt.Printf("Registered %s (%s), minimum role %s.\n", app.Name, clientID, app.MinimumRole)
	fmt.Printf("Connect: %s; callback: %s\n", issuerURL, callback)
	fmt.Printf("Local credentials saved to %s; secret omitted.\n", output)
	return nil
}
