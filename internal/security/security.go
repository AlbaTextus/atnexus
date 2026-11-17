package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidSecret = errors.New("invalid secret")

func RandomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashPassword(password string, cost int) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

const fastSecretPrefix = "sha256:"

// generatedSecret reports whether secret has the shape of RandomToken(32).
// Such secrets carry 256 bits of entropy, so a plain digest is as safe as a
// slow hash; operator-chosen secrets of any other shape keep bcrypt.
func generatedSecret(secret string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(secret)
	return err == nil && len(raw) == 32
}

// HashClientSecret hashes an OAuth client secret. Generated secrets get a
// SHA-256 digest, so token and introspection requests do not pay for bcrypt.
func HashClientSecret(secret string, cost int) (string, error) {
	if generatedSecret(secret) {
		return fastSecretPrefix + HashToken(secret), nil
	}
	return HashPassword(secret, cost)
}

// CheckSecret verifies a client secret against either stored format.
func CheckSecret(hash, secret string) bool {
	if hash == "" {
		return false
	}
	if digest, ok := strings.CutPrefix(hash, fastSecretPrefix); ok {
		return subtle.ConstantTimeCompare([]byte(digest), []byte(HashToken(secret))) == 1
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

// UpgradedSecretHash returns the fast hash for a secret that just verified
// against a bcrypt hash, or "" when the stored hash should stay as it is.
func UpgradedSecretHash(hash, secret string) string {
	if strings.HasPrefix(hash, fastSecretPrefix) || !generatedSecret(secret) {
		return ""
	}
	return fastSecretPrefix + HashToken(secret)
}

func VerifyPKCE(challenge, method, verifier string) bool {
	if !ValidPKCEValue(challenge) || !ValidPKCEValue(verifier) {
		return false
	}
	if method == "" {
		method = "plain"
	}
	var actual string
	switch strings.ToUpper(method) {
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		actual = base64.RawURLEncoding.EncodeToString(sum[:])
	case "PLAIN":
		actual = verifier
	default:
		return false
	}
	if len(actual) != len(challenge) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(challenge)) == 1
}

// ValidPKCEValue enforces RFC 7636's unreserved-character and length rules.
func ValidPKCEValue(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' && r != '_' && r != '~' {
			return false
		}
	}
	return true
}
