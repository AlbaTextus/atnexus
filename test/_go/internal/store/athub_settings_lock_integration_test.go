package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// athubTestConn opens a connection outside the store's small pool, in the same
// test schema.
func athubTestConn(t *testing.T, ctx context.Context, p *Postgres) *pgx.Conn {
	t.Helper()
	conn, err := pgx.ConnectConfig(ctx, p.pool.Config().ConnConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

// stalledATHubCall starts a real admission and holds it open after it has taken
// every admission lock: an uncommitted audit row with the same id makes its own
// audit insert wait. release lets the call finish; holder is the backend the
// call waits on.
func stalledATHubCall(t *testing.T, ctx context.Context, p *Postgres, user uuid.UUID, key ATHubKey) (release func(), holder int32, result <-chan error) {
	t.Helper()
	call := policyCall(user, key, "GET", "/api/v2/content")
	conn := athubTestConn(t, ctx, p)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO athub_api_calls(id,request_id,method,path,port,authenticated)
		VALUES($1,$2,$3,$4,$5,false)`, call.ID, uuid.New(), call.Method, call.Path, call.Port); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.BeginATHubCall(ctx, call) }()
	return func() { tx.Rollback(context.Background()) }, holder, done
}

// athubBlockedBy waits until some backend is blocked by one of holders, either
// on a lock they hold or behind their place in a lock queue, and returns it.
func athubBlockedBy(t *testing.T, ctx context.Context, monitor *pgx.Conn, holders ...int32) int32 {
	t.Helper()
	for {
		var pid int32
		err := monitor.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE pg_blocking_pids(pid) && $1::int[] LIMIT 1`, holders).Scan(&pid)
		if err == nil {
			return pid
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no backend became blocked by %v", holders)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Calls from different accounts of one role keep their admissions overlapping.
// An administrator change must still get its turn once the calls already in
// flight finish, and calls arriving after it must be judged by the new settings.
func TestATHubSettingsChangeNotStarvedByOverlappingCalls(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(context.Context, *Postgres, uuid.UUID) error
		denied error
	}{
		{"key limit", func(ctx context.Context, p *Postgres, actor uuid.UUID) error {
			zero := int64(0)
			_, err := p.SetATHubKeyLimit(ctx, "USER", &zero, actor)
			return err
		}, ErrATHubCallLimit},
		{"policy", func(ctx context.Context, p *Postgres, actor uuid.UUID) error {
			policy, err := p.ATHubPolicy(ctx, "USER")
			if err != nil {
				return err
			}
			for i := range policy.Rules {
				if policy.Rules[i].Operation == "read" {
					policy.Rules[i].Allowed = false
				}
			}
			_, err = p.SetATHubPolicy(ctx, actor, policy)
			return err
		}, ErrATHubPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := athubTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			monitor := athubTestConn(t, ctx, p)
			actor, _ := policyUser(t, p, "SUPER_ADMIN")
			first, firstKey := policyUser(t, p, "USER")
			second, secondKey := policyUser(t, p, "USER")

			// The first call is in flight under the old settings.
			releaseFirst, firstHolder, firstResult := stalledATHubCall(t, ctx, p, first, firstKey)
			defer releaseFirst()
			firstCall := athubBlockedBy(t, ctx, monitor, firstHolder)

			// The administrator change has to wait for it.
			changed := make(chan error, 1)
			go func() { changed <- tc.change(ctx, p, actor) }()
			change := athubBlockedBy(t, ctx, monitor, firstCall)

			// A second account's call arrives before the first one finishes. It has
			// reached either the administrator's lock queue or its own audit insert.
			releaseSecond, secondHolder, secondResult := stalledATHubCall(t, ctx, p, second, secondKey)
			defer releaseSecond()
			athubBlockedBy(t, ctx, monitor, change, secondHolder)

			releaseFirst()
			if err := <-firstResult; err != nil {
				t.Fatalf("call admitted before the change: %v", err)
			}
			// The second call is still in flight. The change must not wait for it.
			select {
			case err := <-changed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("administrator change starved by overlapping admissions")
			}
			// Having waited for the change, the second call sees the new settings.
			athubBlockedBy(t, ctx, monitor, secondHolder)
			releaseSecond()
			if err := <-secondResult; !errors.Is(err, tc.denied) {
				t.Fatalf("call admitted after the change ignored it: %v", err)
			}
		})
	}
}

// A review decision locks the reviewer's policy row before the author's user
// row, the reverse of admission. A policy change that held the role lock while
// waiting for that row would close a cycle through the author's own call.
func TestATHubPolicyChangeDoesNotDeadlockWithReviewDecision(t *testing.T) {
	p := athubTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	monitor := athubTestConn(t, ctx, p)
	actor, _ := policyUser(t, p, "SUPER_ADMIN")
	reviewer, _ := policyUser(t, p, "ADMIN")
	author, authorKey := policyUser(t, p, "ADMIN")
	policy, err := p.ATHubPolicy(ctx, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}

	// The author's call holds the author's user row and waits for the key row.
	keyHolder := athubTestConn(t, ctx, p)
	keyTx, err := keyHolder.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer keyTx.Rollback(context.Background())
	var keyPID int32
	if err := keyTx.QueryRow(ctx, `SELECT pg_backend_pid() FROM athub_api_keys WHERE id=$1 FOR UPDATE`, authorKey.ID).Scan(&keyPID); err != nil {
		t.Fatal(err)
	}
	called := make(chan error, 1)
	go func() { called <- p.BeginATHubCall(ctx, policyCall(author, authorKey, "GET", "/api/v2/content")) }()
	call := athubBlockedBy(t, ctx, monitor, keyPID)

	// The decision holds the ADMIN policy row and waits for the author's user row.
	decided := make(chan error, 1)
	go func() {
		decided <- p.WithCreatorDecision(ctx, reviewer, ATHubReview{UserID: &author, Operation: "upload", Method: "POST", Path: "/api/v2/upload"}, true,
			func(json.RawMessage) error { return nil })
	}()
	decision := athubBlockedBy(t, ctx, monitor, call)

	// The policy change waits for the decision; then the call moves on to settings.
	changed := make(chan error, 1)
	go func() {
		_, err := p.SetATHubPolicy(ctx, actor, policy)
		changed <- err
	}()
	athubBlockedBy(t, ctx, monitor, decision)
	keyTx.Rollback(context.Background())

	for name, result := range map[string]chan error{"call": called, "decision": decided, "policy change": changed} {
		if err := <-result; err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
