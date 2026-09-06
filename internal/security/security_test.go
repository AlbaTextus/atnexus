package security

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestVerifyPKCES256(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if !VerifyPKCE(challenge, "S256", verifier) {
		t.Fatal("expected valid S256 verifier")
	}
	if VerifyPKCE(challenge, "S256", verifier+"x") {
		t.Fatal("expected modified verifier to fail")
	}
	if VerifyPKCE(challenge, "plain", verifier) {
		t.Fatal("expected plain verifier not to match S256 challenge")
	}
}

func TestValidPKCEValue(t *testing.T) {
	if !ValidPKCEValue("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatal("expected valid verifier")
	}
	if ValidPKCEValue("too-short") {
		t.Fatal("short verifier must be rejected")
	}
	if ValidPKCEValue("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=") {
		t.Fatal("padding is not an RFC 7636 character")
	}
}

func TestRandomTokenAndHash(t *testing.T) {
	first, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) < 40 {
		t.Fatal("tokens should be random and sufficiently long")
	}
	if HashToken(first) == HashToken(second) {
		t.Fatal("different tokens must have different hashes")
	}
}
