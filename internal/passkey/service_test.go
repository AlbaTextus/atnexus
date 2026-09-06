package passkey

import (
	"bytes"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

func TestToStoreCredentialPreservesVerificationState(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	publicKey := []byte{1, 2, 3, 4}
	credential := &webauthn.Credential{
		ID: []byte{5, 6, 7}, PublicKey: publicKey, AttestationType: "none",
		Transport:     []protocol.AuthenticatorTransport{protocol.Internal},
		Flags:         webauthn.CredentialFlags{UserPresent: true, UserVerified: true, BackupEligible: true, BackupState: true},
		Authenticator: webauthn.Authenticator{SignCount: 17},
	}
	record := toStoreCredential(uuid.New(), credential, "MacBook Touch ID", now)
	if record.CredentialID != "BQYH" || !bytes.Equal(record.PublicKey, publicKey) {
		t.Fatalf("credential data was not preserved: %#v", record)
	}
	if record.SignCount != 17 || !record.UserPresent || !record.UserVerified || !record.BackupEligible || !record.BackupState {
		t.Fatalf("credential flags were not preserved: %#v", record)
	}
	if record.Name != "MacBook Touch ID" || !record.CreatedAt.Equal(now) {
		t.Fatalf("credential metadata was not preserved: %#v", record)
	}
}
