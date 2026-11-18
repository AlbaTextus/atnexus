package oauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

type memoryStore struct {
	user   store.User
	client store.Client
	code   store.AuthorizationCode
	token  store.Token
}

func (m *memoryStore) Close()                     {}
func (m *memoryStore) Ping(context.Context) error { return nil }
func (m *memoryStore) CreateUser(context.Context, string, string, *string) (store.User, error) {
	return store.User{}, nil
}
func (m *memoryStore) UserByEmail(context.Context, string) (store.User, error) {
	return store.User{}, store.ErrNotFound
}
func (m *memoryStore) UserByID(_ context.Context, id uuid.UUID) (store.User, error) {
	if m.user.ID != id {
		return store.User{}, store.ErrNotFound
	}
	return m.user, nil
}
func (m *memoryStore) CreateSession(context.Context, uuid.UUID, string, time.Time) error { return nil }
func (m *memoryStore) SessionByHash(context.Context, string) (store.Session, error) {
	return store.Session{}, store.ErrNotFound
}
func (m *memoryStore) RevokeSession(context.Context, string) error { return nil }
func (m *memoryStore) PasskeysByUserID(context.Context, uuid.UUID) ([]store.PasskeyCredential, error) {
	return nil, nil
}
func (m *memoryStore) PasskeyByCredentialID(context.Context, string) (store.PasskeyCredential, error) {
	return store.PasskeyCredential{}, store.ErrNotFound
}
func (m *memoryStore) CreatePasskey(context.Context, store.PasskeyCredential) error { return nil }
func (m *memoryStore) UpdatePasskeyUsage(context.Context, string, int64, bool, bool, bool, bool) error {
	return nil
}
func (m *memoryStore) DeletePasskey(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memoryStore) CreatePasskeyCeremony(context.Context, string, *uuid.UUID, string, []byte, time.Time) error {
	return nil
}
func (m *memoryStore) ConsumePasskeyCeremony(context.Context, string, string) (store.PasskeyCeremony, error) {
	return store.PasskeyCeremony{}, store.ErrNotFound
}
func (m *memoryStore) ClientByID(context.Context, string) (store.Client, error) {
	if m.client.ID == "" {
		return store.Client{}, store.ErrNotFound
	}
	return m.client, nil
}
func (m *memoryStore) CreateAuthorizationCode(_ context.Context, in store.NewAuthorizationCode) error {
	m.code = store.AuthorizationCode{CodeHash: in.CodeHash, ClientID: in.ClientID, UserID: in.UserID, RedirectURI: in.RedirectURI, Scopes: in.Scopes, CodeChallenge: in.CodeChallenge, CodeChallengeMethod: in.CodeChallengeMethod, ExpiresAt: in.ExpiresAt}
	return nil
}
func (m *memoryStore) ConsumeAuthorizationCode(_ context.Context, codeHash string) (store.AuthorizationCode, error) {
	if m.code.CodeHash == "" || m.code.CodeHash != codeHash || m.code.UsedAt != nil {
		return store.AuthorizationCode{}, store.ErrNotFound
	}
	now := time.Now()
	m.code.UsedAt = &now
	return m.code, nil
}
func (m *memoryStore) CreateToken(_ context.Context, in store.NewToken) (store.Token, error) {
	m.token = store.Token{AccessTokenHash: in.AccessTokenHash, RefreshTokenHash: in.RefreshTokenHash, ClientID: in.ClientID, UserID: in.UserID, Scopes: in.Scopes, AccessExpiresAt: in.AccessExpiresAt, RefreshExpiresAt: in.RefreshExpiresAt, CreatedAt: time.Now()}
	return m.token, nil
}
func (m *memoryStore) TokenByAccessHash(context.Context, string) (store.Token, error) {
	if m.token.AccessTokenHash == "" {
		return store.Token{}, store.ErrNotFound
	}
	return m.token, nil
}
func (m *memoryStore) TokenByRefreshHash(context.Context, string) (store.Token, error) {
	if m.token.RefreshTokenHash == nil {
		return store.Token{}, store.ErrNotFound
	}
	return m.token, nil
}
func (m *memoryStore) RotateRefreshToken(context.Context, string, store.NewToken) (store.Token, error) {
	return store.Token{}, errors.New("not implemented")
}
func (m *memoryStore) RevokeToken(context.Context, string, string) error { return nil }

func testConfig() config.Config { return config.Config{AuthorizationCodeTTL: 5 * time.Minute} }

func TestMinimumRoleHierarchy(t *testing.T) {
	roles := []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN", "UNKNOWN", ""}
	allowed := map[string]map[string]bool{
		"USER":        {"": true, "USER": true},
		"LEVEL_2":     {"": true, "USER": true, "LEVEL_2": true},
		"ADMIN":       {"": true, "USER": true, "LEVEL_2": true, "ADMIN": true},
		"SUPER_ADMIN": {"": true, "USER": true, "LEVEL_2": true, "ADMIN": true, "SUPER_ADMIN": true},
	}
	for _, role := range roles {
		for _, minimum := range roles {
			for _, active := range []bool{true, false} {
				st := &memoryStore{user: store.User{ID: uuid.New(), Role: role, IsActive: active}}
				svc := NewService(st, testConfig())
				err := svc.checkMinimumRole(context.Background(), store.Client{MinimumRole: minimum}, st.user.ID)
				if (err == nil) != (active && allowed[role][minimum]) {
					t.Errorf("role=%q minimum=%q active=%t: unexpected result %v", role, minimum, active, err)
				}
			}
		}
	}
}

func TestCreateAuthorizationCodeRequiresPKCEForPublicClient(t *testing.T) {
	st := &memoryStore{client: store.Client{ID: "public", IsActive: true, RedirectURIs: []string{"http://localhost/callback"}, AllowedGrantTypes: []string{"authorization_code"}, AllowedScopes: []string{"openid"}}}
	svc := NewService(st, testConfig())
	_, oauthErr := svc.CreateAuthorizationCode(context.Background(), uuid.New(), AuthorizeRequest{ClientID: "public", RedirectURI: "http://localhost/callback", ResponseType: "code", Scope: "openid"})
	if oauthErr == nil || oauthErr.Code != "invalid_request" {
		t.Fatalf("expected PKCE error, got %#v", oauthErr)
	}
}

func TestCreateAndExchangeAuthorizationCode(t *testing.T) {
	secretHash, err := security.HashPassword("secret", 4)
	if err != nil {
		t.Fatal(err)
	}
	st := &memoryStore{client: store.Client{ID: "web", IsActive: true, SecretHash: &secretHash, RedirectURIs: []string{"http://localhost/callback"}, AllowedGrantTypes: []string{"authorization_code"}, AllowedScopes: []string{"email", "openid"}, AccessTokenTTL: 60, RefreshTokenTTL: 120}}
	svc := NewService(st, testConfig())
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	st.user = store.User{ID: uuid.New(), IsActive: true, Role: "USER"}
	code, oauthErr := svc.CreateAuthorizationCode(context.Background(), st.user.ID, AuthorizeRequest{ClientID: "web", RedirectURI: "http://localhost/callback", ResponseType: "code", Scope: "openid email", CodeChallenge: challenge, CodeChallengeMethod: "S256"})
	if oauthErr != nil || code == "" {
		t.Fatalf("create code failed: %#v", oauthErr)
	}
	response, oauthErr := svc.ExchangeCode(context.Background(), "web", "secret", code, "http://localhost/callback", verifier)
	if oauthErr != nil {
		t.Fatalf("exchange failed: %#v", oauthErr)
	}
	if response.AccessToken == "" || response.RefreshToken != "" || response.Scope != "email openid" {
		t.Fatalf("unexpected token response: %#v", response)
	}
}

// Introspection tells a client about its own tokens only. Another client's
// token must look exactly like a token that does not exist.
func TestIntrospectReportsOnlyTheCallersOwnTokens(t *testing.T) {
	secretHash, err := security.HashPassword("secret", 4)
	if err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	for _, tc := range []struct {
		name, issuedTo string
		active         bool
	}{{"own token", "web", true}, {"another client's token", "forum", false}} {
		st := &memoryStore{client: store.Client{ID: "web", IsActive: true, SecretHash: &secretHash}}
		st.token = store.Token{AccessTokenHash: security.HashToken("access"), ClientID: tc.issuedTo, UserID: &user, Scopes: []string{"email", "openid"}, AccessExpiresAt: time.Now().Add(time.Minute), CreatedAt: time.Now()}
		result, oauthErr := NewService(st, testConfig()).Introspect(context.Background(), "web", "secret", "access")
		if oauthErr != nil {
			t.Fatalf("%s: %#v", tc.name, oauthErr)
		}
		if result["active"] != tc.active {
			t.Errorf("%s: active=%v, want %v", tc.name, result["active"], tc.active)
		}
		if tc.active && (result["sub"] != user.String() || result["client_id"] != "web" || result["scope"] != "email openid") {
			t.Errorf("%s: incomplete answer %v", tc.name, result)
		}
		if !tc.active && len(result) != 1 {
			t.Errorf("%s: disclosed %v", tc.name, result)
		}
	}
}

type brokenClients struct{ *memoryStore }

func (brokenClients) ClientByID(context.Context, string) (store.Client, error) {
	return store.Client{}, errors.New("connection refused")
}

func TestAuthorizeReportsAnOutageAsServerError(t *testing.T) {
	service := NewService(brokenClients{&memoryStore{}}, config.Config{})
	_, err := service.ValidateAuthorize(context.Background(), AuthorizeRequest{ClientID: "app", RedirectURI: "https://app.example/callback", ResponseType: "code"})
	if err == nil || err.Code != "server_error" || err.Status != 500 {
		t.Fatalf("a failed client lookup must be a server error, got %v", err)
	}
	service = NewService(&memoryStore{}, config.Config{})
	_, err = service.ValidateAuthorize(context.Background(), AuthorizeRequest{ClientID: "app", RedirectURI: "https://app.example/callback", ResponseType: "code"})
	if err == nil || err.Code != "unauthorized_client" || err.Status != 400 {
		t.Fatalf("an unknown client must stay unauthorized_client, got %v", err)
	}
}
