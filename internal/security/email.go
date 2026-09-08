package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

func NewEmailCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// A server-held key prevents offline guessing of the six-digit code from a DB dump.
func EmailCodeHash(key, challengeID, code string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte("email-verification:" + challengeID + ":" + code))
	return hex.EncodeToString(h.Sum(nil))
}
