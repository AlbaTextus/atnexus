// Package oauth provides account identity and the shared session authority.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/google/uuid"
)

type SessionStore interface {
	SessionByHash(context.Context, string) (store.Session, error)
	UserByID(context.Context, uuid.UUID) (store.User, error)
}

type SessionIdentity struct {
	User    store.User
	Session store.Session
}

type Sessions struct {
	store      SessionStore
	cookieName string
	now        func() time.Time
}

func NewSessions(st SessionStore, cookieName string) *Sessions {
	return &Sessions{store: st, cookieName: cookieName, now: time.Now}
}

func (s *Sessions) Authenticate(r *http.Request) (SessionIdentity, error) {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil || cookie.Value == "" {
		return SessionIdentity{}, auth.ErrUnauthenticated
	}
	lookup := s.store.SessionByHash
	if authoritative, ok := s.store.(interface {
		SessionByHashUncached(context.Context, string) (store.Session, error)
	}); ok {
		lookup = authoritative.SessionByHashUncached
	}
	session, err := lookup(r.Context(), security.HashToken(cookie.Value))
	if errors.Is(err, store.ErrNotFound) {
		return SessionIdentity{}, auth.ErrUnauthenticated
	}
	if err != nil {
		return SessionIdentity{}, fmt.Errorf("session lookup unavailable: %w", err)
	}
	if session.RevokedAt != nil || !session.ExpiresAt.After(s.now()) || session.UserID == uuid.Nil {
		return SessionIdentity{}, auth.ErrUnauthenticated
	}
	user, err := s.store.UserByID(r.Context(), session.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return SessionIdentity{}, auth.ErrUnauthenticated
	}
	if err != nil {
		return SessionIdentity{}, fmt.Errorf("account lookup unavailable: %w", err)
	}
	if !user.IsActive || user.ID != session.UserID {
		return SessionIdentity{}, auth.ErrUnauthenticated
	}
	return SessionIdentity{User: user, Session: session}, nil
}

func (s *Sessions) VerifySession(r *http.Request) (auth.Identity, error) {
	identity, err := s.Authenticate(r)
	if err != nil {
		return auth.Identity{}, err
	}
	return auth.Identity{UserID: identity.User.ID, SessionID: identity.Session.ID, Role: identity.User.Role, ExpiresAt: identity.Session.ExpiresAt}, nil
}
