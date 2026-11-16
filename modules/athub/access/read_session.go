package athubaccess

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ReadSession is bound to a revocable account session, never to a personal API key.
type ReadSession struct {
	UserID    uuid.UUID `json:"user"`
	SessionID uuid.UUID `json:"session"`
	Expires   int64     `json:"exp"`
}

func readSignature(secret, payload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("atnexus:content-read:v1:" + payload))
	return mac.Sum(nil)
}

func IssueReadSession(secret string, userID, sessionID uuid.UUID, sessionExpiry time.Time) (string, time.Time, error) {
	if secret == "" || userID == uuid.Nil || sessionID == uuid.Nil || !sessionExpiry.After(time.Now()) {
		return "", time.Time{}, errors.New("invalid account session")
	}
	expires := time.Now().Add(5 * time.Minute)
	if sessionExpiry.Before(expires) {
		expires = sessionExpiry
	}
	data, err := json.Marshal(ReadSession{userID, sessionID, expires.Unix()})
	if err != nil {
		return "", time.Time{}, err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	return "ats_" + payload + "." + base64.RawURLEncoding.EncodeToString(readSignature(secret, payload)), expires, nil
}

func parseReadSession(secret, token string) (ReadSession, bool) {
	var identity ReadSession
	if secret == "" || len(token) > 1024 || !strings.HasPrefix(token, "ats_") {
		return identity, false
	}
	payload, signature, ok := strings.Cut(token[4:], ".")
	if !ok {
		return identity, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(sig, readSignature(secret, payload)) {
		return identity, false
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(data, &identity) != nil {
		return identity, false
	}
	return identity, identity.UserID != uuid.Nil && identity.SessionID != uuid.Nil && identity.Expires > time.Now().Unix()
}

func (a *Access) EnableReadSessions(secret string) { a.readSecret = secret }
