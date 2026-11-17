package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/google/uuid"
)

type sessionFixture struct {
	session store.Session
	user    store.User
	err     error
	hash    string
	looked  bool
}

func (s *sessionFixture) SessionByHash(_ context.Context, hash string) (store.Session, error) {
	s.hash = hash
	s.looked = true
	return s.session, s.err
}
func (s *sessionFixture) UserByID(context.Context, uuid.UUID) (store.User, error) { return s.user, nil }

func TestSharedSessionAuthority(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "revoked", "expired", "disabled", "wrong owner", "not found", "database unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			id := uuid.New()
			fixture := &sessionFixture{session: store.Session{ID: uuid.New(), UserID: id, ExpiresAt: time.Now().Add(time.Hour)}, user: store.User{ID: id, IsActive: true, Role: "USER"}}
			request := httptest.NewRequest("GET", "http://localhost:8005/api/v5/novelinfo", nil)
			request.AddCookie(&http.Cookie{Name: "session", Value: "opaque-session"})
			wantInvalid := true
			switch scenario {
			case "valid":
				wantInvalid = false
			case "missing":
				request.Header.Del("Cookie")
			case "revoked":
				now := time.Now()
				fixture.session.RevokedAt = &now
			case "expired":
				fixture.session.ExpiresAt = time.Now().Add(-time.Second)
			case "disabled":
				fixture.user.IsActive = false
			case "wrong owner":
				fixture.user.ID = uuid.New()
			case "not found":
				fixture.err = store.ErrNotFound
			case "database unavailable":
				fixture.err = errors.New("database offline")
				wantInvalid = false
			}
			identity, err := NewSessions(fixture, "session").VerifySession(request)
			if scenario == "valid" {
				if err != nil || identity.UserID != id || identity.SessionID != fixture.session.ID || identity.Role != "USER" || !fixture.looked || fixture.hash != security.HashToken("opaque-session") {
					t.Fatalf("identity=%+v err=%v", identity, err)
				}
			} else if err == nil || errors.Is(err, auth.ErrUnauthenticated) != wantInvalid {
				t.Fatalf("err=%v wantInvalid=%v", err, wantInvalid)
			}
		})
	}
}
