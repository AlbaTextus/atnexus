// Register the local image host without reseeding users or rotating other applications.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	appURL := flag.String("app-url", "http://127.0.0.1:8080", "Image host origin")
	backendURL := flag.String("backend-url", "http://127.0.0.1:8081", "Internal OAuth API origin")
	output := flag.String("output", "../imgbed/.dev.vars", "Local image host environment file")
	flag.Parse()
	if err := run(*appURL, *backendURL, *output); err != nil {
		fmt.Fprintln(os.Stderr, "Image host registration failed:", err)
		os.Exit(1)
	}
}

func run(appURL, backendURL, output string) error {
	_ = godotenv.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := store.NewPostgres(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer st.Close()
	cipher, err := security.NewSecretCipher(os.Getenv("CLIENT_SECRET_ENCRYPTION_KEY"))
	if err != nil {
		return err
	}
	const id = "albatextus-imgbed"
	appURL = strings.TrimRight(appURL, "/")
	app, err := st.ApplicationByID(ctx, id, uuid.Nil, true)
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
		encrypted, err := cipher.Encrypt(id, secret)
		if err != nil {
			return err
		}
		app, err = st.CreateApplication(ctx, store.Application{
			ID: id, Name: "ALBATEXTUS 图床", HomepageURL: appURL,
			Description: "用户头像、背景图、小说封面、插图及论坛图片托管",
			IconURL:     appURL + "/static/media/logo.png", MinimumRole: "USER", IsActive: true,
			RedirectURIs:  []string{appURL + "/api/auth/oauth/callback"},
			AllowedScopes: []string{"profile", "email"}, AllowedGrantTypes: []string{"authorization_code"},
		}, &hash, &encrypted)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if app.IsPublic || app.Name != "ALBATEXTUS 图床" {
			return errors.New("client ID already belongs to a different application")
		}
		encrypted, err := st.ApplicationSecret(ctx, id, uuid.Nil, true)
		if err != nil {
			return err
		}
		if encrypted == nil {
			return errors.New("existing application has no recoverable secret")
		}
		secret, err = cipher.Decrypt(id, *encrypted)
		if err != nil {
			return err
		}
		app.HomepageURL = appURL
		app.RedirectURIs = []string{appURL + "/api/auth/oauth/callback"}
		if _, err = st.UpdateApplication(ctx, app, uuid.Nil, true); err != nil {
			return err
		}
	}
	values := map[string]string{}
	if existing, err := godotenv.Read(output); err == nil {
		values = existing
	} else if !os.IsNotExist(err) {
		return err
	}
	issuer := strings.TrimRight(os.Getenv("ISSUER_URL"), "/")
	if issuer == "" {
		issuer = "http://localhost:3288"
	}
	values["OAUTH_ENABLED"] = "true"
	values["OAUTH_ISSUER"] = issuer
	values["OAUTH_BACKEND_URL"] = strings.TrimRight(backendURL, "/")
	values["OAUTH_APP_URL"] = appURL
	values["OAUTH_CLIENT_ID"] = id
	values["OAUTH_CLIENT_SECRET"] = secret
	contents, err := godotenv.Marshal(values)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(output, []byte(contents+"\n"), 0600); err != nil {
		return err
	}
	fmt.Printf("Registered %s (%s). Local credentials saved to %s; secret omitted.\n", app.Name, id, output)
	return nil
}
