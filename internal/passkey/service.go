package passkey

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AlbaTextus/connect/internal/config"
	"github.com/AlbaTextus/connect/internal/security"
	"github.com/AlbaTextus/connect/internal/store"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

const (
	ceremonyRegistration = "registration"
	ceremonyLogin        = "login"
)

type Service struct {
	store store.Store
	web   *webauthn.WebAuthn
	cfg   config.Config
	now   func() time.Time
}

type User struct {
	Record      store.User
	Credentials []webauthn.Credential
}

func (u *User) WebAuthnID() []byte   { return u.Record.ID[:] }
func (u *User) WebAuthnName() string { return u.Record.Email }
func (u *User) WebAuthnDisplayName() string {
	if u.Record.DisplayName != nil && strings.TrimSpace(*u.Record.DisplayName) != "" {
		return *u.Record.DisplayName
	}
	return u.Record.Email
}
func (u *User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

func NewService(st store.Store, cfg config.Config) (*Service, error) {
	web, err := webauthn.New(&webauthn.Config{
		RPID: cfg.WebAuthnRPID, RPDisplayName: cfg.WebAuthnRPName, RPOrigins: cfg.WebAuthnRPOrigins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationPreferred},
	})
	if err != nil {
		return nil, fmt.Errorf("configure webauthn: %w", err)
	}
	return &Service{store: st, web: web, cfg: cfg, now: time.Now}, nil
}

func (s *Service) BeginRegistration(ctx context.Context, userRecord store.User) (protocol.PublicKeyCredentialCreationOptions, string, error) {
	user, err := s.loadUser(ctx, userRecord)
	if err != nil {
		return protocol.PublicKeyCredentialCreationOptions{}, "", err
	}
	exclusions := make([]protocol.CredentialDescriptor, 0, len(user.Credentials))
	for _, credential := range user.Credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}
	creation, session, err := s.web.BeginRegistration(user, webauthn.WithExclusions(exclusions))
	if err != nil {
		return protocol.PublicKeyCredentialCreationOptions{}, "", fmt.Errorf("begin passkey registration: %w", err)
	}
	data, err := json.Marshal(session)
	if err != nil {
		return protocol.PublicKeyCredentialCreationOptions{}, "", fmt.Errorf("encode passkey ceremony: %w", err)
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return protocol.PublicKeyCredentialCreationOptions{}, "", err
	}
	if err := s.store.CreatePasskeyCeremony(ctx, security.HashToken(token), &userRecord.ID, ceremonyRegistration, data, s.now().Add(s.cfg.PasskeyChallengeTTL)); err != nil {
		return protocol.PublicKeyCredentialCreationOptions{}, "", err
	}
	return creation.Response, token, nil
}

func (s *Service) FinishRegistration(ctx context.Context, userRecord store.User, token, name string, request *http.Request) (store.PasskeyCredential, error) {
	ceremony, err := s.store.ConsumePasskeyCeremony(ctx, security.HashToken(token), ceremonyRegistration)
	if errors.Is(err, store.ErrNotFound) {
		return store.PasskeyCredential{}, fmt.Errorf("passkey ceremony is invalid or expired")
	}
	if err != nil {
		return store.PasskeyCredential{}, err
	}
	if ceremony.UserID == nil || *ceremony.UserID != userRecord.ID {
		return store.PasskeyCredential{}, fmt.Errorf("passkey ceremony does not belong to this user")
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(ceremony.SessionData, &session); err != nil {
		return store.PasskeyCredential{}, fmt.Errorf("decode passkey ceremony: %w", err)
	}
	user, err := s.loadUser(ctx, userRecord)
	if err != nil {
		return store.PasskeyCredential{}, err
	}
	credential, err := s.web.FinishRegistration(user, session, request)
	if err != nil {
		return store.PasskeyCredential{}, fmt.Errorf("verify passkey registration: %w", err)
	}
	credentialRecord := toStoreCredential(userRecord.ID, credential, name, s.now())
	if err := s.store.CreatePasskey(ctx, credentialRecord); err != nil {
		return store.PasskeyCredential{}, err
	}
	return credentialRecord, nil
}

func (s *Service) BeginLogin(ctx context.Context) (protocol.PublicKeyCredentialRequestOptions, string, error) {
	assertion, session, err := s.web.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		return protocol.PublicKeyCredentialRequestOptions{}, "", fmt.Errorf("begin passkey login: %w", err)
	}
	data, err := json.Marshal(session)
	if err != nil {
		return protocol.PublicKeyCredentialRequestOptions{}, "", fmt.Errorf("encode passkey ceremony: %w", err)
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return protocol.PublicKeyCredentialRequestOptions{}, "", err
	}
	if err := s.store.CreatePasskeyCeremony(ctx, security.HashToken(token), nil, ceremonyLogin, data, s.now().Add(s.cfg.PasskeyChallengeTTL)); err != nil {
		return protocol.PublicKeyCredentialRequestOptions{}, "", err
	}
	return assertion.Response, token, nil
}

func (s *Service) FinishLogin(ctx context.Context, token string, request *http.Request) (store.User, store.PasskeyCredential, error) {
	ceremony, err := s.store.ConsumePasskeyCeremony(ctx, security.HashToken(token), ceremonyLogin)
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, store.PasskeyCredential{}, fmt.Errorf("passkey ceremony is invalid or expired")
	}
	if err != nil {
		return store.User{}, store.PasskeyCredential{}, err
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(ceremony.SessionData, &session); err != nil {
		return store.User{}, store.PasskeyCredential{}, fmt.Errorf("decode passkey ceremony: %w", err)
	}
	var resolved *User
	user, credential, err := s.web.FinishPasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		var record store.User
		if len(userHandle) == 16 {
			id, parseErr := uuid.FromBytes(userHandle)
			if parseErr == nil {
				record, err = s.store.UserByID(ctx, id)
			}
		}
		if record.ID == uuid.Nil {
			stored, lookupErr := s.store.PasskeyByCredentialID(ctx, base64.RawURLEncoding.EncodeToString(rawID))
			if lookupErr != nil {
				return nil, lookupErr
			}
			record, err = s.store.UserByID(ctx, stored.UserID)
		}
		if err != nil {
			return nil, err
		}
		resolved, err = s.loadUser(ctx, record)
		if err != nil {
			return nil, err
		}
		return resolved, nil
	}, session, request)
	if err != nil {
		return store.User{}, store.PasskeyCredential{}, fmt.Errorf("verify passkey login: %w", err)
	}
	resolvedRecord, ok := user.(*User)
	if !ok || resolvedRecord == nil {
		return store.User{}, store.PasskeyCredential{}, fmt.Errorf("passkey user is invalid")
	}
	if err := s.store.UpdatePasskeyUsage(ctx, base64.RawURLEncoding.EncodeToString(credential.ID), int64(credential.Authenticator.SignCount), credential.Flags.UserPresent, credential.Flags.UserVerified, credential.Flags.BackupEligible, credential.Flags.BackupState); err != nil {
		return store.User{}, store.PasskeyCredential{}, err
	}
	stored, err := s.store.PasskeyByCredentialID(ctx, base64.RawURLEncoding.EncodeToString(credential.ID))
	if err != nil {
		return store.User{}, store.PasskeyCredential{}, err
	}
	return resolvedRecord.Record, stored, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]store.PasskeyCredential, error) {
	return s.store.PasskeysByUserID(ctx, userID)
}
func (s *Service) Delete(ctx context.Context, userID, credentialID uuid.UUID) error {
	return s.store.DeletePasskey(ctx, userID, credentialID)
}

func (s *Service) loadUser(ctx context.Context, record store.User) (*User, error) {
	if !record.IsActive {
		return nil, fmt.Errorf("user account is inactive")
	}
	stored, err := s.store.PasskeysByUserID(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	credentials := make([]webauthn.Credential, 0, len(stored))
	for _, credential := range stored {
		decoded, err := base64.RawURLEncoding.DecodeString(credential.CredentialID)
		if err != nil {
			return nil, fmt.Errorf("decode stored passkey id: %w", err)
		}
		transports := make([]protocol.AuthenticatorTransport, 0, len(credential.Transports))
		for _, transport := range credential.Transports {
			transports = append(transports, protocol.AuthenticatorTransport(transport))
		}
		credentials = append(credentials, webauthn.Credential{ID: decoded, PublicKey: credential.PublicKey, AttestationType: credential.AttestationType, Transport: transports, Flags: webauthn.CredentialFlags{UserPresent: credential.UserPresent, UserVerified: credential.UserVerified, BackupEligible: credential.BackupEligible, BackupState: credential.BackupState}, Authenticator: webauthn.Authenticator{SignCount: uint32(credential.SignCount)}})
	}
	return &User{Record: record, Credentials: credentials}, nil
}

func toStoreCredential(userID uuid.UUID, credential *webauthn.Credential, name string, now time.Time) store.PasskeyCredential {
	if strings.TrimSpace(name) == "" {
		name = "通行密钥"
	}
	transports := make([]string, 0, len(credential.Transport))
	for _, transport := range credential.Transport {
		transports = append(transports, string(transport))
	}
	return store.PasskeyCredential{ID: uuid.New(), UserID: userID, CredentialID: base64.RawURLEncoding.EncodeToString(credential.ID), PublicKey: credential.PublicKey, AttestationType: credential.AttestationType, Transports: transports, SignCount: int64(credential.Authenticator.SignCount), UserPresent: credential.Flags.UserPresent, UserVerified: credential.Flags.UserVerified, BackupEligible: credential.Flags.BackupEligible, BackupState: credential.Flags.BackupState, Name: name, CreatedAt: now}
}
