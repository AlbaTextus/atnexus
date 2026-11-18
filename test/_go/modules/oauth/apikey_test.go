package oauth

import (
	"context"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

type apiKeyFixture struct {
	hash     string
	identity store.ATHubIdentity
}

func (s *apiKeyFixture) ResolveATHubKey(_ context.Context, hash string) (store.ATHubIdentity, error) {
	s.hash = hash
	return s.identity, nil
}

func TestExternalKeyAuthorityKeepsOwnership(t *testing.T) {
	fixture := &apiKeyFixture{identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: true}}
	identity, err := NewAPIKeys(fixture).ResolveAPIKey(context.Background(), "test-external-key")
	if err != nil || fixture.hash != security.HashToken("test-external-key") || identity.KeyID != fixture.identity.KeyID || identity.UserID != fixture.identity.UserID || !identity.Active {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
}
