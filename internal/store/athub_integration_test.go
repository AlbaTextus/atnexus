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
	sql, err := os.ReadFile("../../prisma/migrations/20261110000000_athub_user_keys/migration.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
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
	if created != 20 || limited != 4 {
		t.Fatalf("created=%d limited=%d", created, limited)
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
