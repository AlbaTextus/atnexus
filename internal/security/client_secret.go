package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// SecretCipher stores recoverable client credentials separately from their bcrypt hashes.
type SecretCipher struct{ aead cipher.AEAD }

func NewSecretCipher(encodedKey string) (*SecretCipher, error) {
	key, err := hex.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("CLIENT_SECRET_ENCRYPTION_KEY must contain 64 hexadecimal characters (32 random bytes)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretCipher{aead: aead}, nil
}

func (c *SecretCipher) Encrypt(clientID, secret string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(secret), []byte("albatextus/client-secret/v1:"+clientID))
	return "v1." + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *SecretCipher) Decrypt(clientID, encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, "v1.") {
		return "", errors.New("unsupported client secret encryption format")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encrypted, "v1."))
	if err != nil || len(data) < c.aead.NonceSize()+c.aead.Overhead() {
		return "", errors.New("invalid encrypted client secret")
	}
	nonce, ciphertext := data[:c.aead.NonceSize()], data[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, nonce, ciphertext, []byte("albatextus/client-secret/v1:"+clientID))
	if err != nil {
		return "", errors.New("unable to decrypt client secret")
	}
	return string(plain), nil
}
