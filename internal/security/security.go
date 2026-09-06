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

func CheckSecret(hash, secret string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
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
