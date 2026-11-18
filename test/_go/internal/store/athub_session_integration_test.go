package store

import (
	"context"
	"errors"
	"testing"
	"time"

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
			usage, err := st.ATHubUsage(ctx, userID)
			if err != nil || usage.Total != 0 || len(usage.Endpoints) != 0 ||
				usage.Quota == nil || usage.Quota.RemainingCalls == nil || *usage.Quota.RemainingCalls != 0 {
				t.Fatalf("session traffic changed API key usage: %+v err=%v", usage, err)
			}
			calls, err := st.ListATHubCalls(ctx, userID, 10, 0)
			if err != nil || len(calls) != 0 {
				t.Fatalf("session traffic in API key history: %+v err=%v", calls, err)
			}
		})
	}
}

func TestPurgeATHubCallLogKeepsKeyCallsIntegration(t *testing.T) {
	st := athubTestStore(t)
	ctx := context.Background()
	user := uuid.New()
	if _, err := st.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	key, err := st.CreateATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user, Name: "ATHub", TokenHash: uuid.NewString(), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	insert := func(keyID *uuid.UUID, userID *uuid.UUID, age string) {
		t.Helper()
		if _, err := st.pool.Exec(ctx, `INSERT INTO athub_api_calls(id,user_id,key_id,request_id,method,path,port,authenticated,started_at)
			VALUES($1,$2,$3,$4,'GET','/api/v2/content',8003,true,now()-$5::interval)`, uuid.New(), userID, keyID, uuid.New(), age); err != nil {
			t.Fatal(err)
		}
	}
	insert(nil, nil, "40 days")       // anonymous, old
	insert(nil, &user, "40 days")     // browser session, old
	insert(nil, &user, "1 day")       // browser session, recent
	insert(&key.ID, &user, "40 days") // personal key, old
	removed, err := st.PurgeATHubCallLog(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil || removed != 2 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	var keyless, keyed int
	if err := st.pool.QueryRow(ctx, `SELECT COUNT(*) FILTER(WHERE key_id IS NULL),COUNT(*) FILTER(WHERE key_id IS NOT NULL)
		FROM athub_api_calls`).Scan(&keyless, &keyed); err != nil || keyless != 1 || keyed != 1 {
		t.Fatalf("keyless=%d keyed=%d err=%v", keyless, keyed, err)
	}
}
