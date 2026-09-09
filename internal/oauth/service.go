package oauth

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/AlbaTextus/connect/internal/config"
	"github.com/AlbaTextus/connect/internal/security"
	"github.com/AlbaTextus/connect/internal/store"
	"github.com/google/uuid"
)

type Error struct {
	Code        string `json:"error"`
	Description string `json:"error_description,omitempty"`
	Status      int    `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Description }
func oauthError(code, description string, status int) *Error {
	return &Error{Code: code, Description: description, Status: status}
}

type AuthorizeRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type Service struct {
	store store.Store
	cfg   config.Config
	now   func() time.Time
}

func NewService(s store.Store, cfg config.Config) *Service {
	return &Service{store: s, cfg: cfg, now: time.Now}
}

func (s *Service) ValidateAuthorize(ctx context.Context, in AuthorizeRequest) (store.Client, *Error) {
	if in.ResponseType != "code" {
		return store.Client{}, oauthError("unsupported_response_type", "response_type must be code", 400)
	}
	if in.ClientID == "" || in.RedirectURI == "" {
		return store.Client{}, oauthError("invalid_request", "client_id and redirect_uri are required", 400)
	}
	parsedRedirect, err := url.ParseRequestURI(in.RedirectURI)
	if err != nil || (parsedRedirect.Scheme != "https" && parsedRedirect.Scheme != "http") || parsedRedirect.Hostname() == "" || parsedRedirect.User != nil || parsedRedirect.Fragment != "" {
		return store.Client{}, oauthError("invalid_request", "redirect_uri is invalid", 400)
	}
	client, err := s.store.ClientByID(ctx, in.ClientID)
	if errors.Is(err, store.ErrNotFound) || err != nil || !client.IsActive {
		return store.Client{}, oauthError("unauthorized_client", "client is unknown or inactive", 400)
	}
	if !contains(client.RedirectURIs, in.RedirectURI) {
		return store.Client{}, oauthError("invalid_request", "redirect_uri is not registered", 400)
	}
	if !contains(client.AllowedGrantTypes, "authorization_code") {
		return store.Client{}, oauthError("unauthorized_client", "authorization_code is not enabled for this client", 400)
	}
	_, ok := requestedScopes(in.Scope, client.AllowedScopes)
	if !ok {
		return store.Client{}, oauthError("invalid_scope", "requested scope is not allowed", 400)
	}
	if client.IsPublic() && (in.CodeChallenge == "" || !strings.EqualFold(in.CodeChallengeMethod, "S256")) {
		return store.Client{}, oauthError("invalid_request", "public clients must use S256 PKCE", 400)
	}
	if in.CodeChallenge != "" {
		if !security.ValidPKCEValue(in.CodeChallenge) || !strings.EqualFold(in.CodeChallengeMethod, "S256") {
			return store.Client{}, oauthError("invalid_request", "code_challenge must use S256", 400)
		}
	}
	return client, nil
}

func (s *Service) CreateAuthorizationCode(ctx context.Context, userID uuid.UUID, in AuthorizeRequest) (string, *Error) {
	code, record, err := s.PrepareAuthorizationCode(ctx, userID, in)
	if err != nil {
		return "", err
	}
	if dbErr := s.store.CreateAuthorizationCode(ctx, record); dbErr != nil {
		return "", oauthError("server_error", "could not persist authorization code", 500)
	}
	return code, nil
}

// PrepareAuthorizationCode validates a request without issuing a usable code.
// Consent stores the prepared record in the same transaction as the decision.
func (s *Service) PrepareAuthorizationCode(ctx context.Context, userID uuid.UUID, in AuthorizeRequest) (string, store.NewAuthorizationCode, *Error) {
	client, err := s.ValidateAuthorize(ctx, in)
	if err != nil {
		return "", store.NewAuthorizationCode{}, err
	}
	if roleErr := s.checkMinimumRole(ctx, client, userID); roleErr != nil {
		return "", store.NewAuthorizationCode{}, roleErr
	}
	code, genErr := security.RandomToken(32)
	if genErr != nil {
		return "", store.NewAuthorizationCode{}, oauthError("server_error", "could not create authorization code", 500)
	}
	challenge, method := optionalString(in.CodeChallenge), optionalString(in.CodeChallengeMethod)
	return code, store.NewAuthorizationCode{CodeHash: security.HashToken(code), ClientID: client.ID, UserID: userID, RedirectURI: in.RedirectURI, Scopes: normalizeScopes(in.Scope), CodeChallenge: challenge, CodeChallengeMethod: method, ExpiresAt: s.now().Add(s.cfg.AuthorizationCodeTTL)}, nil
}

func (s *Service) ValidateConsent(ctx context.Context, userID uuid.UUID, in AuthorizeRequest) (store.Client, *Error) {
	client, err := s.ValidateAuthorize(ctx, in)
	if err != nil {
		return store.Client{}, err
	}
	if err := s.checkMinimumRole(ctx, client, userID); err != nil {
		return store.Client{}, err
	}
	return client, nil
}

func (s *Service) ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI, verifier string) (TokenResponse, *Error) {
	client, authErr := s.authenticateClient(ctx, clientID, clientSecret, false)
	if authErr != nil {
		return TokenResponse{}, authErr
	}
	if !contains(client.AllowedGrantTypes, "authorization_code") {
		return TokenResponse{}, oauthError("unauthorized_client", "grant type is not enabled", 400)
	}
	if code == "" || redirectURI == "" {
		return TokenResponse{}, oauthError("invalid_request", "code and redirect_uri are required", 400)
	}
	record, err := s.store.ConsumeAuthorizationCode(ctx, security.HashToken(code))
	if errors.Is(err, store.ErrNotFound) {
		return TokenResponse{}, oauthError("invalid_grant", "authorization code is invalid or expired", 400)
	}
	if err != nil {
		return TokenResponse{}, oauthError("server_error", "could not consume authorization code", 500)
	}
	if record.ClientID != client.ID || record.RedirectURI != redirectURI {
		return TokenResponse{}, oauthError("invalid_grant", "authorization code does not belong to this client", 400)
	}
	if record.CodeChallenge != nil && (!security.ValidPKCEValue(verifier) || !security.VerifyPKCE(*record.CodeChallenge, valueOrDefault(record.CodeChallengeMethod, "S256"), verifier)) {
		return TokenResponse{}, oauthError("invalid_grant", "code_verifier is invalid", 400)
	}
	return s.issueToken(ctx, client, &record.UserID, record.Scopes, true)
}

func (s *Service) Refresh(ctx context.Context, clientID, clientSecret, refresh, scope string) (TokenResponse, *Error) {
	client, authErr := s.authenticateClient(ctx, clientID, clientSecret, false)
	if authErr != nil {
		return TokenResponse{}, authErr
	}
	if !contains(client.AllowedGrantTypes, "refresh_token") {
		return TokenResponse{}, oauthError("unauthorized_client", "refresh_token is not enabled", 400)
	}
	if refresh == "" {
		return TokenResponse{}, oauthError("invalid_request", "refresh_token is required", 400)
	}
	old, err := s.store.TokenByRefreshHash(ctx, security.HashToken(refresh))
	if errors.Is(err, store.ErrNotFound) {
		return TokenResponse{}, oauthError("invalid_grant", "refresh token is invalid or expired", 400)
	}
	if err != nil {
		return TokenResponse{}, oauthError("server_error", "could not read refresh token", 500)
	}
	if old.ClientID != client.ID {
		return TokenResponse{}, oauthError("invalid_grant", "refresh token does not belong to this client", 400)
	}
	if old.UserID != nil {
		if roleErr := s.checkMinimumRole(ctx, client, *old.UserID); roleErr != nil {
			return TokenResponse{}, roleErr
		}
	}
	scopes := old.Scopes
	if scope != "" {
		var ok bool
		scopes, ok = requestedScopes(scope, old.Scopes)
		if !ok {
			return TokenResponse{}, oauthError("invalid_scope", "requested scope exceeds the original grant", 400)
		}
	}
	access, genErr := security.RandomToken(32)
	if genErr != nil {
		return TokenResponse{}, oauthError("server_error", "could not create access token", 500)
	}
	newRefresh, genErr := security.RandomToken(32)
	if genErr != nil {
		return TokenResponse{}, oauthError("server_error", "could not create refresh token", 500)
	}
	refreshExpiry := s.now().Add(time.Duration(client.RefreshTokenTTL) * time.Second)
	input := store.NewToken{AccessTokenHash: security.HashToken(access), RefreshTokenHash: stringPtr(security.HashToken(newRefresh)), ClientID: client.ID, UserID: old.UserID, Scopes: scopes, AccessExpiresAt: s.now().Add(time.Duration(client.AccessTokenTTL) * time.Second), RefreshExpiresAt: &refreshExpiry}
	if _, err = s.store.RotateRefreshToken(ctx, security.HashToken(refresh), input); errors.Is(err, store.ErrNotFound) {
		return TokenResponse{}, oauthError("invalid_grant", "refresh token is invalid or expired", 400)
	} else if err != nil {
		return TokenResponse{}, oauthError("server_error", "could not rotate refresh token", 500)
	}
	return TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int64(client.AccessTokenTTL), RefreshToken: newRefresh, Scope: strings.Join(scopes, " ")}, nil
}

func (s *Service) ClientCredentials(ctx context.Context, clientID, secret, scope string) (TokenResponse, *Error) {
	client, authErr := s.authenticateClient(ctx, clientID, secret, true)
	if authErr != nil {
		return TokenResponse{}, authErr
	}
	if !contains(client.AllowedGrantTypes, "client_credentials") {
		return TokenResponse{}, oauthError("unauthorized_client", "client_credentials is not enabled", 400)
	}
	scopes, ok := requestedScopes(scope, client.AllowedScopes)
	if !ok {
		return TokenResponse{}, oauthError("invalid_scope", "requested scope is not allowed", 400)
	}
	return s.issueToken(ctx, client, nil, scopes, false)
}

func (s *Service) issueToken(ctx context.Context, client store.Client, userID *uuid.UUID, scopes []string, withRefresh bool) (TokenResponse, *Error) {
	if userID != nil {
		if roleErr := s.checkMinimumRole(ctx, client, *userID); roleErr != nil {
			return TokenResponse{}, roleErr
		}
	}
	access, err := security.RandomToken(32)
	if err != nil {
		return TokenResponse{}, oauthError("server_error", "could not create access token", 500)
	}
	var refresh string
	var refreshHash *string
	var refreshExpiry *time.Time
	if withRefresh && contains(client.AllowedGrantTypes, "refresh_token") {
		refresh, err = security.RandomToken(32)
		if err != nil {
			return TokenResponse{}, oauthError("server_error", "could not create refresh token", 500)
		}
		h := security.HashToken(refresh)
		refreshHash = &h
		expiry := s.now().Add(time.Duration(client.RefreshTokenTTL) * time.Second)
		refreshExpiry = &expiry
	}
	_, dbErr := s.store.CreateToken(ctx, store.NewToken{AccessTokenHash: security.HashToken(access), RefreshTokenHash: refreshHash, ClientID: client.ID, UserID: userID, Scopes: scopes, AccessExpiresAt: s.now().Add(time.Duration(client.AccessTokenTTL) * time.Second), RefreshExpiresAt: refreshExpiry})
	if dbErr != nil {
		return TokenResponse{}, oauthError("server_error", "could not persist token", 500)
	}
	return TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int64(client.AccessTokenTTL), RefreshToken: refresh, Scope: strings.Join(scopes, " ")}, nil
}

func (s *Service) Revoke(ctx context.Context, clientID, secret, tokenValue string) *Error {
	if _, authErr := s.authenticateClient(ctx, clientID, secret, false); authErr != nil {
		return authErr
	}
	if tokenValue == "" {
		return nil
	}
	err := s.store.RevokeToken(ctx, security.HashToken(tokenValue), clientID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return oauthError("server_error", "could not revoke token", 500)
	}
	return nil
}

func (s *Service) Introspect(ctx context.Context, clientID, secret, tokenValue string) (map[string]any, *Error) {
	if _, authErr := s.authenticateClient(ctx, clientID, secret, false); authErr != nil {
		return nil, authErr
	}
	result := map[string]any{"active": false}
	if tokenValue == "" {
		return result, nil
	}
	t, err := s.store.TokenByAccessHash(ctx, security.HashToken(tokenValue))
	if errors.Is(err, store.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, oauthError("server_error", "could not introspect token", 500)
	}
	result["active"] = true
	result["client_id"] = t.ClientID
	result["scope"] = strings.Join(t.Scopes, " ")
	result["exp"] = t.AccessExpiresAt.Unix()
	result["iat"] = t.CreatedAt.Unix()
	result["token_type"] = "Bearer"
	if t.UserID != nil {
		result["sub"] = t.UserID.String()
	}
	return result, nil
}

func (s *Service) AuthenticateAccessToken(ctx context.Context, raw string) (store.Token, *Error) {
	if raw == "" {
		return store.Token{}, oauthError("invalid_token", "bearer token is required", 401)
	}
	t, err := s.store.TokenByAccessHash(ctx, security.HashToken(raw))
	if errors.Is(err, store.ErrNotFound) {
		return store.Token{}, oauthError("invalid_token", "bearer token is invalid or expired", 401)
	}
	if err != nil {
		return store.Token{}, oauthError("server_error", "could not validate access token", 500)
	}
	return t, nil
}

func (s *Service) authenticateClient(ctx context.Context, id, secret string, requireSecret bool) (store.Client, *Error) {
	if id == "" {
		return store.Client{}, oauthError("invalid_client", "client authentication is required", 401)
	}
	c, err := s.store.ClientByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) || err != nil || !c.IsActive {
		return store.Client{}, oauthError("invalid_client", "client authentication failed", 401)
	}
	if requireSecret && c.IsPublic() {
		return store.Client{}, oauthError("invalid_client", "this grant requires a confidential client", 401)
	}
	if !c.IsPublic() && !security.CheckSecret(valueOrDefault(c.SecretHash, ""), secret) {
		return store.Client{}, oauthError("invalid_client", "client authentication failed", 401)
	}
	return c, nil
}

func (s *Service) checkMinimumRole(ctx context.Context, client store.Client, userID uuid.UUID) *Error {
	user, err := s.store.UserByID(ctx, userID)
	if err != nil || !user.IsActive || !store.MeetsMinimumRole(user.Role, client.MinimumRole) {
		return oauthError("access_denied", "your account does not meet this application's minimum role", 403)
	}
	return nil
}

func requestedScopes(raw string, allowed []string) ([]string, bool) {
	scopes := normalizeScopes(raw)
	for _, scope := range scopes {
		if !contains(allowed, scope) {
			return nil, false
		}
	}
	return scopes, true
}
func normalizeScopes(raw string) []string {
	parts := strings.Fields(raw)
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func stringPtr(value string) *string { return &value }
func valueOrDefault(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}
