package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/AlbaTextus/oauth/internal/store"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		panic("DATABASE_URL is required")
	}
	st, err := store.NewPostgres(ctx, databaseURL)
	if err != nil {
		panic(err)
	}
	defer st.Close()
	secretCipher, err := security.NewSecretCipher(strings.TrimSpace(os.Getenv("CLIENT_SECRET_ENCRYPTION_KEY")))
	if err != nil {
		panic(err)
	}
	clientID := getenv("SEED_CLIENT_ID", "albatextus-web")
	name := getenv("SEED_CLIENT_NAME", "ALBATEXTUS Web")
	secret := getenv("SEED_CLIENT_SECRET", "development-secret-change-me")
	secretHash, err := security.HashPassword(secret, 12)
	if err != nil {
		panic(err)
	}
	encryptedSecret, err := secretCipher.Encrypt(clientID, secret)
	if err != nil {
		panic(err)
	}
	accessTTL := getInt("SEED_ACCESS_TOKEN_TTL", 900)
	refreshTTL := getInt("SEED_REFRESH_TOKEN_TTL", 2592000)
	// Client seeding is intentionally SQL-backed; Prisma remains the schema/migration owner.
	if err := seedClient(ctx, st, clientID, name, secretHash, encryptedSecret, splitEnv("SEED_CLIENT_REDIRECT_URIS", "http://localhost:3000/auth/callback"), splitEnv("SEED_CLIENT_GRANT_TYPES", "authorization_code,refresh_token,client_credentials"), splitEnv("SEED_CLIENT_SCOPES", "openid,profile,email,offline_access"), accessTTL, refreshTTL); err != nil {
		panic(err)
	}
	fmt.Printf("seeded confidential OAuth client %q\nclient_secret=%s\n", clientID, secret)
	adminEmail := getenv("SEED_ADMIN_EMAIL", "admin@example.com")
	adminPassword := getenv("SEED_ADMIN_PASSWORD", "Admin123!ChangeMe")
	adminHash, err := security.HashPassword(adminPassword, 12)
	if err != nil {
		panic(err)
	}
	adminName := getenv("SEED_ADMIN_NAME", "Administrator")
	admin := (*string)(&adminName)
	if _, err := st.UpsertUser(ctx, store.UserSeed{Email: strings.ToLower(strings.TrimSpace(adminEmail)), PasswordHash: adminHash, DisplayName: admin, Role: "ADMIN"}); err != nil {
		panic(err)
	}
	fmt.Printf("seeded admin user %q\nadmin_password=%s\n", adminEmail, adminPassword)
}

func seedClient(ctx context.Context, st *store.Postgres, id, name, secretHash, encryptedSecret string, redirects, grants, scopes []string, accessTTL, refreshTTL int) error {
	// Keep this helper in the command so production deployments can use a separate secret manager.
	return st.UpsertClient(ctx, store.ClientSeed{ID: id, Name: name, SecretHash: secretHash, SecretCiphertext: &encryptedSecret, RedirectURIs: redirects, AllowedGrantTypes: grants, AllowedScopes: scopes, AccessTokenTTL: accessTTL, RefreshTokenTTL: refreshTTL})
}
func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func getInt(key string, fallback int) int {
	value, err := strconv.Atoi(getenv(key, strconv.Itoa(fallback)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
func splitEnv(key, fallback string) []string {
	raw := getenv(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}
