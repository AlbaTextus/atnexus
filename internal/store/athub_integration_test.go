package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/AlbaTextus/oauth/internal/security"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func athubTestStore(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("OAUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set OAUTH_TEST_DATABASE_URL to test ATHub account tables")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := "athub_auth_test_" + uuid.New().String()
	schema := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close(ctx)
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Minimal existing users contract; apply the real new migration unchanged.
	if _, err := pool.Exec(ctx, `CREATE TABLE users(id uuid PRIMARY KEY,is_active boolean NOT NULL DEFAULT true)`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{"20261110000000_athub_user_keys", "20261110010000_athub_single_key", "20261110020000_athub_admin_disable"} {
		sql, err := os.ReadFile("../../prisma/migrations/" + migration + "/migration.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
	}
	return &Postgres{pool: pool}
}

func TestATHubKeysAndAccountingIntegration(t *testing.T) {
	p := athubTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.CheckATHubSchema(ctx); err != nil {
		t.Fatal(err)
	}
	user, other := uuid.New(), uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1),($2)`, user, other); err != nil {
		t.Fatal(err)
	}
	token := "ath_integration_key"
	key, err := p.CreateATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user, Name: "collector", TokenHash: security.HashToken(token), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := p.ResolveATHubKey(ctx, security.HashToken(token))
	if err != nil || !identity.Active || identity.UserID != user || identity.KeyID != key.ID {
		t.Fatalf("identity=%+v error=%v", identity, err)
	}
	if _, err := p.ResolveATHubKey(ctx, security.HashToken("unknown")); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown key resolved")
	}
	if err := p.RevokeATHubKey(ctx, other, key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-user revocation accepted")
	}
	otherKeys, err := p.ListATHubKeys(ctx, other)
	if err != nil || len(otherKeys) != 0 {
		t.Fatal("cross-user key list leaked")
	}
	statuses := []int{200, 401, 0}
	for _, status := range statuses {
		call := ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &key.ID, RequestID: uuid.New(), Method: "GET", Path: "/api/v2/content", Port: 8003, Authenticated: status != 401}
		if err := p.BeginATHubCall(ctx, call); err != nil {
			t.Fatal(err)
		}
		if status != 0 {
			if err := p.FinishATHubCall(ctx, call.ID, status, 7, 123); err != nil {
				t.Fatal(err)
			}
		}
	}
	usage, err := p.ATHubUsage(ctx, user)
	if err != nil || usage.Total != 3 || usage.Success != 1 || usage.Failed != 1 || usage.Pending != 1 || len(usage.Endpoints) != 3 {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
	for _, group := range usage.Endpoints {
		if group.Port != 8003 || group.Path != "/api/v2/content" {
			t.Fatal("bad usage group")
		}
	}
	otherUsage, err := p.ATHubUsage(ctx, other)
	if err != nil || otherUsage.Total != 0 {
		t.Fatal("cross-user statistics leaked")
	}
	calls, err := p.ListATHubCalls(ctx, user, 2, 0)
	if err != nil || len(calls) != 2 {
		t.Fatal("call pagination failed")
	}
	otherCalls, err := p.ListATHubCalls(ctx, other, 10, 0)
	if err != nil || len(otherCalls) != 0 {
		t.Fatal("cross-user call history leaked")
	}
	keys, err := p.ListATHubKeys(ctx, user)
	if err != nil || len(keys) != 1 || keys[0].TotalCalls != 3 || keys[0].LastUsedAt == nil {
		t.Fatalf("keys=%+v err=%v", keys, err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE users SET is_active=false WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	identity, err = p.ResolveATHubKey(ctx, security.HashToken(token))
	if err != nil || identity.Active {
		t.Fatal("disabled user retained access")
	}
	if _, err := p.pool.Exec(ctx, `UPDATE users SET is_active=true WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE athub_api_keys SET expires_at=now()-interval '1 minute' WHERE id=$1`, key.ID); err != nil {
		t.Fatal(err)
	}
	identity, err = p.ResolveATHubKey(ctx, security.HashToken(token))
	if err != nil || identity.Active {
		t.Fatal("expired key retained access")
	}
	if _, err := p.pool.Exec(ctx, `UPDATE athub_api_keys SET expires_at=NULL WHERE id=$1`, key.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.RevokeATHubKey(ctx, user, key.ID); err != nil {
		t.Fatal(err)
	}
	identity, err = p.ResolveATHubKey(ctx, security.HashToken(token))
	if err != nil || identity.Active {
		t.Fatal("revoked key retained access")
	}
	if _, err := p.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM athub_api_calls WHERE user_id IS NULL AND key_id IS NULL`).Scan(&retained); err != nil || retained != 3 {
		t.Fatalf("audit records lost: %d %v", retained, err)
	}
}

func TestATHubConcurrentKeyLimitIntegration(t *testing.T) {
	p := athubTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user := uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 24)
	var workers sync.WaitGroup
	for range 24 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			id := uuid.New()
			_, err := p.CreateATHubKey(ctx, ATHubKey{ID: id, UserID: user, Name: "concurrent", TokenHash: security.HashToken(id.String()), Prefix: "ath_test"})
			results <- err
		}()
	}
	workers.Wait()
	close(results)
	created, limited := 0, 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrATHubKeyLimit):
			limited++
		default:
			t.Fatal(err)
		}
	}
	if created != 1 || limited != 23 {
		t.Fatalf("created=%d limited=%d", created, limited)
	}
}

func TestATHubSingleKeyLifecycleIntegration(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	user := uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	makeKey := func() ATHubKey {
		id := uuid.New()
		return ATHubKey{ID: id, UserID: user, Name: "ATHub", TokenHash: security.HashToken(id.String()), Prefix: "ath_test"}
	}
	var workers sync.WaitGroup
	results := make(chan bool, 24)
	for range 24 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			key, created, err := p.EnsureATHubKey(ctx, makeKey())
			if err != nil {
				t.Error(err)
			}
			if key.ExpiresAt != nil {
				t.Error("key should never expire")
			}
			results <- created
		}()
	}
	workers.Wait()
	close(results)
	created := 0
	for result := range results {
		if result {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created=%d", created)
	}
	keys, err := p.ListATHubKeys(ctx, user)
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys=%+v err=%v", keys, err)
	}
	oldID := keys[0].ID
	call := ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &oldID, RequestID: uuid.New(), Method: "GET", Path: "/api/v2/content", Port: 8003, Authenticated: true}
	if err := p.BeginATHubCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	if err := p.FinishATHubCall(ctx, call.ID, 200, 1, 10); err != nil {
		t.Fatal(err)
	}
	if err := p.RevokeATHubKey(ctx, user, oldID); err != nil {
		t.Fatal(err)
	}
	same, wasCreated, err := p.EnsureATHubKey(ctx, makeKey())
	if err != nil || wasCreated || same.ID != oldID || same.RevokedAt == nil {
		t.Fatal("disabled key was replaced or enabled")
	}
	newKey := makeKey()
	replacement, err := p.ResetATHubKey(ctx, newKey)
	if err != nil || replacement.ID == oldID || replacement.RevokedAt != nil || replacement.ExpiresAt != nil {
		t.Fatalf("replacement=%+v err=%v", replacement, err)
	}
	oldIdentity, err := p.ResolveATHubKey(ctx, security.HashToken(oldID.String()))
	if err != nil || oldIdentity.Active {
		t.Fatal("old key still active")
	}
	newIdentity, err := p.ResolveATHubKey(ctx, newKey.TokenHash)
	if err != nil || !newIdentity.Active {
		t.Fatal("new key not active")
	}
	keys, err = p.ListATHubKeys(ctx, user)
	if err != nil || len(keys) != 1 || keys[0].ID != newKey.ID {
		t.Fatal("multiple current keys")
	}
	usage, err := p.ATHubUsage(ctx, user)
	if err != nil || usage.Total != 1 || usage.Success != 1 {
		t.Fatal("history lost during reset")
	}
	// The database invariant must also reject writes bypassing the service.
	_, err = p.pool.Exec(ctx, `INSERT INTO athub_api_keys(id,user_id,name,token_hash,prefix) VALUES($1,$2,'bypass',$3,'ath_test')`, uuid.New(), user, security.HashToken(uuid.NewString()))
	if !isUniqueViolation(err) {
		t.Fatalf("database allowed a second current key: %v", err)
	}
}

func TestATHubSingleKeyMigrationRetainsHistory(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	if _, err := p.pool.Exec(ctx, `ALTER TABLE athub_api_keys DROP COLUMN is_current`); err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	older, newer := uuid.New(), uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO athub_api_keys(id,user_id,name,token_hash,prefix,created_at,expires_at,revoked_at)
		VALUES($1,$3,'usable',$4,'ath_old',now()-interval '1 day',now()+interval '1 day',NULL),
		      ($2,$3,'disabled',$5,'ath_new',now(),NULL,now())`, older, newer, user, security.HashToken(older.String()), security.HashToken(newer.String())); err != nil {
		t.Fatal(err)
	}
	call := ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &newer, RequestID: uuid.New(), Method: "GET", Path: "/api/v2/content", Port: 8003}
	if err := p.BeginATHubCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../prisma/migrations/20261110010000_athub_single_key/migration.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.pool.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	keys, err := p.ListATHubKeys(ctx, user)
	if err != nil || len(keys) != 1 || keys[0].ID != older || keys[0].ExpiresAt != nil {
		t.Fatalf("keys=%+v err=%v", keys, err)
	}
	var retained int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM athub_api_keys WHERE user_id=$1`, user).Scan(&retained); err != nil || retained != 2 {
		t.Fatal("migration removed keys")
	}
	calls, err := p.ListATHubCalls(ctx, user, 10, 0)
	if err != nil || len(calls) != 1 || calls[0].KeyID == nil || *calls[0].KeyID != newer {
		t.Fatal("migration removed historical call attribution")
	}
}

func TestATHubUsageRangeIntegration(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	user, other := uuid.New(), uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1),($2)`, user, other); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	until := from.Add(24 * time.Hour)
	for _, c := range []struct {
		user         uuid.UUID
		at           time.Time
		port, status int
	}{
		{user, from.Add(-time.Second), 8003, 200},
		{user, from, 8003, 200},
		{user, from.Add(30 * time.Minute), 8003, 500},
		{user, from.Add(time.Hour), 8004, 0},
		{user, until, 8003, 200},
		{other, from, 8003, 200},
	} {
		var status *int
		if c.status != 0 {
			status = &c.status
		}
		if _, err := p.pool.Exec(ctx, `INSERT INTO athub_api_calls(id,user_id,request_id,method,path,port,authenticated,status,started_at)
			VALUES($1,$2,$3,'GET','/api/v2/content',$4,true,$5,$6)`, uuid.New(), c.user, uuid.New(), c.port, status, c.at); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := p.ATHubUsageRange(ctx, user, from, until, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Total != 3 || usage.Success != 1 || usage.Failed != 1 || usage.Pending != 1 || len(usage.Trend) != 2 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	if usage.Trend[0].Calls != 2 || usage.Trend[0].Port != 8003 || !usage.Trend[0].StartedAt.Equal(from) || usage.Trend[1].Port != 8004 {
		t.Fatalf("unexpected buckets: %+v", usage.Trend)
	}
	daily, err := p.ATHubUsageRange(ctx, user, from, until, 86400)
	if err != nil || len(daily.Trend) != 2 {
		t.Fatalf("daily=%+v err=%v", daily, err)
	}
}

func TestATHubAdminBanBetweenLookupAndDispatch(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	user := uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	key, err := p.CreateATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user, Name: "ATHub", TokenHash: security.HashToken("test"), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := p.ResolveATHubKey(ctx, key.TokenHash)
	if err != nil || !identity.Active {
		t.Fatal("initial authentication failed")
	}
	if _, err := p.pool.Exec(ctx, `UPDATE users SET athub_disabled=true WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	call := ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &key.ID, RequestID: uuid.New(), Method: "GET", Path: "/api/v2/content", Port: 8003, Authenticated: true}
	if err := p.BeginATHubCall(ctx, call); !errors.Is(err, ErrATHubAccessDenied) {
		t.Fatalf("stale authentication accepted: %v", err)
	}
	if err := p.FinishATHubCall(ctx, call.ID, 401, 1, 0); err != nil {
		t.Fatal(err)
	}
	calls, err := p.ListATHubCalls(ctx, user, 10, 0)
	if err != nil || len(calls) != 1 || calls[0].Authenticated {
		t.Fatal("denied call audit was not preserved")
	}
	if _, err := p.ResetATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user}); !errors.Is(err, ErrATHubAdminDisabled) {
		t.Fatal("reset bypassed admin restriction")
	}
	existing, created, err := p.EnsureATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user})
	if err != nil || created || !existing.AdminDisabled || existing.ID != key.ID {
		t.Fatal("provisioning bypassed admin restriction")
	}
	emptyUser := uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id,athub_disabled) VALUES($1,true)`, emptyUser); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.EnsureATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: emptyUser}); !errors.Is(err, ErrATHubAdminDisabled) {
		t.Fatal("blocked user without key obtained one")
	}
}
