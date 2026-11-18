package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/google/uuid"
)

func policyUser(t *testing.T, p *Postgres, role string) (uuid.UUID, ATHubKey) {
	t.Helper()
	user := uuid.New()
	if _, err := p.pool.Exec(context.Background(), `INSERT INTO users(id,role,display_name) VALUES($1,$2,$3)`, user, role, role); err != nil {
		t.Fatal(err)
	}
	key, err := p.CreateATHubKey(context.Background(), ATHubKey{ID: uuid.New(), UserID: user, Name: "test", TokenHash: security.HashToken(uuid.NewString()), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	return user, key
}
func policyCall(user uuid.UUID, key ATHubKey, method, path string) ATHubCall {
	return ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &key.ID, RequestID: uuid.New(), Method: method, Path: path, Port: 8003, Authenticated: true}
}
func TestATHubDefaultOperationPermissions(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		user, key := policyUser(t, p, role)
		policy, err := p.ATHubPolicy(ctx, role)
		if err != nil {
			t.Fatal(err)
		}
		if policy.ReviewRequired != (role == "LEVEL_2") {
			t.Fatal("wrong default review requirement")
		}
		for _, tc := range []struct {
			method, path string
			minimum      int
		}{{"GET", "/api/v2/content", 1}, {"POST", "/api/v2/upload", 2}, {"PUT", "/api/v2/content", 2}, {"POST", "/api/v3/reviews", 3}, {"DELETE", "/api/v2/novelinfo", 4}} {
			err := p.BeginATHubCall(ctx, policyCall(user, key, tc.method, tc.path))
			if RoleLevel(role) >= tc.minimum && err != nil || RoleLevel(role) < tc.minimum && !errors.Is(err, ErrATHubPermissionDenied) {
				t.Fatalf("role=%s method=%s error=%v", role, tc.method, err)
			}
		}
	}
}
func TestATHubPermissionsIgnoreLegacyQuotasAndKeepPolicyRevision(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	admin, _ := policyUser(t, p, "SUPER_ADMIN")
	user, key := policyUser(t, p, "USER")
	policy, err := p.ATHubPolicy(ctx, "USER")
	if err != nil {
		t.Fatal(err)
	}
	two := int64(2)
	for i := range policy.Rules {
		if policy.Rules[i].Operation == "read" {
			policy.Rules[i].TotalCallLimit = &two
		}
	}
	updated, err := p.SetATHubPolicy(ctx, admin, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range updated.Rules {
		if rule.TotalCallLimit != nil {
			t.Fatal("obsolete operation limit returned after save")
		}
	}
	if _, err := p.SetATHubPolicy(ctx, admin, policy); !errors.Is(err, ErrConflict) {
		t.Fatal("stale policy overwrote current configuration", err)
	}
	if _, err := p.SetATHubPolicy(ctx, user, updated); !errors.Is(err, ErrUserForbidden) {
		t.Fatal("user changed configuration", err)
	}
	results := make(chan error, 20)
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- p.BeginATHubCall(ctx, policyCall(user, key, "GET", "/api/v2/content"))
		}()
	}
	workers.Wait()
	close(results)
	allowed := 0
	for err := range results {
		if err == nil {
			allowed++
		} else {
			t.Fatal(err)
		}
	}
	if allowed != 20 {
		t.Fatalf("obsolete operation quota still enforced: allowed=%d", allowed)
	}
	// Exact endpoint rules can disable one read without disabling every GET.
	for i := range updated.Rules {
		updated.Rules[i].TotalCallLimit = nil
	}
	updated.Rules = append(updated.Rules, ATHubAccessRule{Operation: "read", Method: "GET", Path: "/api/v2/tags", Allowed: false})
	if _, err := p.SetATHubPolicy(ctx, admin, updated); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, policyCall(user, key, "GET", "/api/v2/tags")); !errors.Is(err, ErrATHubPermissionDenied) {
		t.Fatal("endpoint denial bypassed", err)
	}
	if err := p.BeginATHubCall(ctx, policyCall(user, key, "GET", "/api/v2/content")); err != nil {
		t.Fatal("endpoint denial leaked", err)
	}
	updated, err = p.ATHubPolicy(ctx, "USER")
	if err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	updated.Rules = append(updated.Rules, ATHubAccessRule{Operation: "read", Method: "GET", Path: "/api/v2/toc", Allowed: true, TotalCallLimit: &one})
	if _, err := p.SetATHubPolicy(ctx, admin, updated); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, policyCall(user, key, "GET", "/api/v2/toc")); err != nil {
		t.Fatal(err)
	}
	// Older stored caps must also stop limiting access without an admin resave.
	if _, err := p.pool.Exec(ctx, `UPDATE athub_access_rules SET total_call_limit=0 WHERE role='USER' AND operation='read'`); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, policyCall(user, key, "GET", "/api/v2/toc")); err != nil {
		t.Fatal("obsolete endpoint quota still enforced", err)
	}
	// Signed library reads keep permission checks without per-operation caps.
	session := uuid.New()
	if _, err := p.pool.Exec(ctx, `CREATE TABLE user_sessions(id uuid PRIMARY KEY,user_id uuid NOT NULL,expires_at timestamptz NOT NULL,revoked_at timestamptz);
		INSERT INTO user_sessions(id,user_id,expires_at) VALUES('`+session.String()+`','`+user.String()+`',now()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	sessionCall := func() ATHubCall {
		call := policyCall(user, key, "GET", "/api/v2/toc")
		call.KeyID = nil
		call.SessionID = &session
		return call
	}
	if err := p.BeginATHubCall(ctx, sessionCall()); err != nil {
		t.Fatal("key quota leaked into session quota", err)
	}
	if err := p.BeginATHubCall(ctx, sessionCall()); err != nil {
		t.Fatal("obsolete signed session quota still enforced", err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE athub_access_rules SET allowed=false WHERE role='USER' AND method='GET' AND path='/api/v2/toc'`); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, sessionCall()); !errors.Is(err, ErrATHubPermissionDenied) {
		t.Fatal("signed session bypassed endpoint permission", err)
	}
}
