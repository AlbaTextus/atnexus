package oauth

import (
	"context"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/pkg/auth"
)

type APIKeyStore interface {
	ResolveATHubKey(context.Context, string) (store.ATHubIdentity, error)
}

// APIKeys is the authority for existing external credentials. The persistence
// adapter retains the existing key hashes and account-disable checks.
type APIKeys struct{ store APIKeyStore }

func NewAPIKeys(st APIKeyStore) *APIKeys { return &APIKeys{store: st} }

func (s *APIKeys) ResolveAPIKey(ctx context.Context, raw string) (auth.APIKeyIdentity, error) {
	identity, err := s.store.ResolveATHubKey(ctx, security.HashToken(raw))
	if err != nil {
		return auth.APIKeyIdentity{}, err
	}
	return auth.APIKeyIdentity{KeyID: identity.KeyID, UserID: identity.UserID, Active: identity.Active}, nil
}
