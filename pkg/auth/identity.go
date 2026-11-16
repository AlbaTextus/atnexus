// Package auth defines the identity contract shared by the backend modules.
package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

var ErrUnauthenticated = errors.New("session is invalid or expired")
var ErrForbidden = errors.New("permission denied")

type Identity struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Role      string
	ExpiresAt time.Time
}

type Verifier interface {
	VerifySession(*http.Request) (Identity, error)
}

type APIKeyIdentity struct {
	KeyID  uuid.UUID
	UserID uuid.UUID
	Active bool
}

type APIKeyVerifier interface {
	ResolveAPIKey(context.Context, string) (APIKeyIdentity, error)
}

type identityKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}
