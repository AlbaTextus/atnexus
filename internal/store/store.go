package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
)

type User struct {
	Bio             string
	NumericID       int
	ID              uuid.UUID
	Email           string
	PasswordHash    string
	DisplayName     *string
	AvatarSource    string
	AvatarURL       string
	CustomAvatarURL string
	Role            string
	IsActive        bool
	CreatedAt       time.Time
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type PasskeyCredential struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	CredentialID    string
	PublicKey       []byte
	AttestationType string
	Transports      []string
	SignCount       int64
	UserPresent     bool
	UserVerified    bool
	BackupEligible  bool
	BackupState     bool
	Name            string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}

type PasskeyCeremony struct {
	ID          uuid.UUID
	TokenHash   string
	UserID      *uuid.UUID
	Kind        string
	SessionData []byte
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

type Client struct {
	ID                string
	Name              string
	HomepageURL       string
	Description       string
	IconURL           string
	UpdatedAt         time.Time
	SecretHash        *string
	RedirectURIs      []string
	AllowedGrantTypes []string
	AllowedScopes     []string
	AccessTokenTTL    int
	RefreshTokenTTL   int
	IsActive          bool
	MinimumRole       string
}

type ClientSeed struct {
	ID                string
	Name              string
	SecretHash        string
	SecretCiphertext  *string
	RedirectURIs      []string
	AllowedGrantTypes []string
	AllowedScopes     []string
	AccessTokenTTL    int
	RefreshTokenTTL   int
}

type UserSeed struct {
	Email        string
	PasswordHash string
	DisplayName  *string
	Role         string
}

func (c Client) IsPublic() bool { return c.SecretHash == nil || *c.SecretHash == "" }

type AuthorizationCode struct {
	ID                  uuid.UUID
	CodeHash            string
	ClientID            string
	UserID              uuid.UUID
	RedirectURI         string
	Scopes              []string
	CodeChallenge       *string
	CodeChallengeMethod *string
	ExpiresAt           time.Time
	UsedAt              *time.Time
}

type Token struct {
	ID               uuid.UUID
	AccessTokenHash  string
	RefreshTokenHash *string
	ClientID         string
	UserID           *uuid.UUID
	Scopes           []string
	AccessExpiresAt  time.Time
	RefreshExpiresAt *time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
}

type NewAuthorizationCode struct {
	CodeHash            string
	ClientID            string
	UserID              uuid.UUID
	RedirectURI         string
	Scopes              []string
	CodeChallenge       *string
	CodeChallengeMethod *string
	ExpiresAt           time.Time
}

type NewToken struct {
	AccessTokenHash  string
	RefreshTokenHash *string
	ClientID         string
	UserID           *uuid.UUID
	Scopes           []string
	AccessExpiresAt  time.Time
	RefreshExpiresAt *time.Time
}

// Store contains the persistence operations needed by the OAuth service.
// Implementations can be swapped in tests without coupling handlers to SQL.
type Store interface {
	Close()
	Ping(context.Context) error
	CreateUser(context.Context, string, string, *string) (User, error)
	UserByEmail(context.Context, string) (User, error)
	UserByID(context.Context, uuid.UUID) (User, error)
	CreateSession(context.Context, uuid.UUID, string, time.Time) error
	SessionByHash(context.Context, string) (Session, error)
	RevokeSession(context.Context, string) error
	PasskeysByUserID(context.Context, uuid.UUID) ([]PasskeyCredential, error)
	PasskeyByCredentialID(context.Context, string) (PasskeyCredential, error)
	CreatePasskey(context.Context, PasskeyCredential) error
	UpdatePasskeyUsage(context.Context, string, int64, bool, bool, bool, bool) error
	DeletePasskey(context.Context, uuid.UUID, uuid.UUID) error
	CreatePasskeyCeremony(context.Context, string, *uuid.UUID, string, []byte, time.Time) error
	ConsumePasskeyCeremony(context.Context, string, string) (PasskeyCeremony, error)
	ClientByID(context.Context, string) (Client, error)
	CreateAuthorizationCode(context.Context, NewAuthorizationCode) error
	ConsumeAuthorizationCode(context.Context, string) (AuthorizationCode, error)
	CreateToken(context.Context, NewToken) (Token, error)
	TokenByAccessHash(context.Context, string) (Token, error)
	TokenByRefreshHash(context.Context, string) (Token, error)
	RotateRefreshToken(context.Context, string, NewToken) (Token, error)
	RevokeToken(context.Context, string, string) error
}
