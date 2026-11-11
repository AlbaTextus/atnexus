package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestATHubSessionAccountingIntegration(t *testing.T) {
	st := athubTestStore(t)
	ctx := context.Background()
	userID, sessionID := uuid.New(), uuid.New()
	_, err := st.pool.Exec(ctx, `CREATE TABLE user_sessions(id uuid PRIMARY KEY,user_id uuid NOT NULL,expires_at timestamptz NOT NULL,revoked_at timestamptz);
		INSERT INTO users(id) VALUES('`+userID.String()+`');
		INSERT INTO user_sessions(id,user_id,expires_at) VALUES('`+sessionID.String()+`','`+userID.String()+`',now()+interval '1 hour')`)
	if err != nil {
		t.Fatal(err)
	}
	// Key quotas must not restrict the library's signed session reads.
	zero := int64(0)
	if _, err := st.SetATHubKeyLimit(ctx, "USER", &zero, userID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, setup string
		allowed     bool
	}{
		{"active without key", "", true},
		{"administrator restriction", `UPDATE users SET athub_disabled=true`, false},
		{"account ban", `UPDATE users SET athub_disabled=false,is_active=false`, false},
		{"logout", `UPDATE users SET is_active=true; UPDATE user_sessions SET revoked_at=now()`, false},
		{"session expired", `UPDATE user_sessions SET revoked_at=NULL,expires_at=now()-interval '1 second'`, false},
		{"session deleted", `DELETE FROM user_sessions`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != "" {
				if _, err := st.pool.Exec(ctx, tc.setup); err != nil {
					t.Fatal(err)
				}
			}
			call := ATHubCall{ID: uuid.New(), RequestID: uuid.New(), UserID: &userID, SessionID: &sessionID, Method: "GET", Path: "/api/v2/novelinfo", Port: 8003, Authenticated: true}
			err := st.BeginATHubCall(ctx, call)
			if tc.allowed && err != nil || !tc.allowed && !errors.Is(err, ErrATHubAccessDenied) {
				t.Fatalf("allowed=%v error=%v", tc.allowed, err)
			}
			var authenticated bool
			var keyID *uuid.UUID
			if err := st.pool.QueryRow(ctx, `SELECT authenticated,key_id FROM athub_api_calls WHERE id=$1`, call.ID).Scan(&authenticated, &keyID); err != nil || authenticated != tc.allowed || keyID != nil {
				t.Fatalf("invalid session audit: %v", err)
			}
		})
	}
}
