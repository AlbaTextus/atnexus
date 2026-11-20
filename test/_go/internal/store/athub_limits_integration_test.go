package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/google/uuid"
)

func TestATHubKeyTotalLimitIntegration(t *testing.T) {
	p := athubTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user, actor := uuid.New(), uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	key, err := p.CreateATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user, Name: "quota", TokenHash: security.HashToken(uuid.NewString()), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	makeCall := func() ATHubCall {
		return ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &key.ID, RequestID: uuid.New(), Method: "GET", Path: "/api/v2/content", Port: 8003, Authenticated: true}
	}
	limits, err := p.ATHubKeyLimits(ctx)
	if err != nil || len(limits) != 4 {
		t.Fatalf("limits=%+v error=%v", limits, err)
	}
	for _, limit := range limits {
		if limit.TotalCallLimit != nil {
			t.Fatal("migration must leave all levels unlimited")
		}
	}
	for i, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		if limits[i].Role != role {
			t.Fatalf("unexpected limit order: %+v", limits)
		}
	}
	// Historical successful, failed and pending calls all consume the total.
	for _, status := range []int{200, 500, 0} {
		call := makeCall()
		if err := p.BeginATHubCall(ctx, call); err != nil {
			t.Fatal(err)
		}
		if status != 0 {
			if err := p.FinishATHubCall(ctx, call.ID, status, 1, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	limit := int64(5)
	if _, err := p.SetATHubKeyLimit(ctx, "USER", &limit, actor); err != nil {
		t.Fatal(err)
	}
	// Race the final two slots; every denial must also retain an audit record.
	results := make(chan error, 20)
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- p.BeginATHubCall(ctx, makeCall())
		}()
	}
	workers.Wait()
	close(results)
	admitted, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, ErrATHubCallLimit):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if admitted != 2 || rejected != 18 {
		t.Fatalf("admitted=%d rejected=%d", admitted, rejected)
	}
	var total, authenticated int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE authenticated) FROM athub_api_calls WHERE key_id=$1`, key.ID).Scan(&total, &authenticated); err != nil || total != 23 || authenticated != 5 {
		t.Fatalf("audit total=%d admitted=%d error=%v", total, authenticated, err)
	}
	// Each level has its own setting; a committed role change applies immediately.
	if _, err := p.pool.Exec(ctx, `UPDATE users SET role='ADMIN' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, makeCall()); err != nil {
		t.Fatal("USER limit leaked into ADMIN level", err)
	}
	zero := int64(0)
	if _, err := p.SetATHubKeyLimit(ctx, "ADMIN", &zero, actor); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, makeCall()); !errors.Is(err, ErrATHubCallLimit) {
		t.Fatal("zero limit did not deny calls", err)
	}
	if _, err := p.SetATHubKeyLimit(ctx, "ADMIN", nil, actor); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, makeCall()); err != nil {
		t.Fatal("clearing the limit did not restore access", err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE users SET role='LEVEL_2' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, makeCall()); err != nil {
		t.Fatal("LT2 must default to unlimited", err)
	}
	if _, err := p.SetATHubKeyLimit(ctx, "LEVEL_2", &zero, actor); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, makeCall()); !errors.Is(err, ErrATHubCallLimit) {
		t.Fatal("LT2 limit not applied", err)
	}
	if _, err := p.SetATHubKeyLimit(ctx, "LEVEL_2", nil, actor); err != nil {
		t.Fatal(err)
	}
	if err := p.BeginATHubCall(ctx, makeCall()); err != nil {
		t.Fatal("LT2 unlimited access was not restored", err)
	}
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		if _, err := p.SetATHubKeyLimit(ctx, role, &limit, actor); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.SetATHubKeyLimit(ctx, "LT2", &limit, actor); !errors.Is(err, ErrInvalidATHubKeyLimit) {
		t.Fatal("unknown role accepted", err)
	}
}

func TestATHubUsageQuotaIntegration(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	user := uuid.New()
	if _, err := p.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, user); err != nil {
		t.Fatal(err)
	}
	key, err := p.CreateATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user, Name: "quota", TokenHash: uuid.NewString(), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	// Calls outside every chart range still consume the current key's allowance.
	for _, status := range []int{200, 500, 0} {
		call := ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &key.ID, RequestID: uuid.New(), Method: "GET", Path: "/api/v2/content", Port: 8003, Authenticated: true}
		if err := p.BeginATHubCall(ctx, call); err != nil {
			t.Fatal(err)
		}
		if status != 0 {
			if err := p.FinishATHubCall(ctx, call.ID, status, 1, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Rejected requests remain in the audit log but do not consume admitted-call quota.
	if err := p.BeginATHubCall(ctx, ATHubCall{ID: uuid.New(), UserID: &user, KeyID: &key.ID, RequestID: uuid.New(), Method: "GET", Path: "/api/v2/content", Port: 8003}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE athub_api_calls SET started_at=now()-interval '60 days' WHERE key_id=$1`, key.ID); err != nil {
		t.Fatal(err)
	}
	check := func(limit *int64, wantRemaining *int64) {
		t.Helper()
		if _, err := p.SetATHubKeyLimit(ctx, "USER", limit, uuid.New()); err != nil {
			t.Fatal(err)
		}
		for _, days := range []int{1, 7, 30} {
			until := time.Now().UTC()
			usage, err := p.ATHubUsageRange(ctx, user, until.Add(-time.Duration(days)*24*time.Hour), until, 3600)
			if err != nil {
				t.Fatal(err)
			}
			if usage.Total != 0 || usage.Quota == nil {
				t.Fatalf("range total=%d quota=%+v", usage.Total, usage.Quota)
			}
			quota := usage.Quota
			if (quota.TotalCallLimit == nil) != (limit == nil) || (quota.RemainingCalls == nil) != (wantRemaining == nil) {
				t.Fatalf("unexpected quota: %+v", quota)
			}
			if limit != nil && (*quota.TotalCallLimit != *limit || *quota.RemainingCalls != *wantRemaining) {
				t.Fatalf("limit=%d remaining=%d want=%d", *quota.TotalCallLimit, *quota.RemainingCalls, *wantRemaining)
			}
		}
	}
	five, two, zero := int64(5), int64(2), int64(0)
	check(nil, nil)
	check(&five, &two)
	check(&two, &zero)
	check(&zero, &zero)
	// Rotation starts a new allowance, without counting the old key's history.
	if _, err := p.ResetATHubKey(ctx, ATHubKey{ID: uuid.New(), UserID: user, Name: "new quota", TokenHash: uuid.NewString(), Prefix: "ath_new"}); err != nil {
		t.Fatal(err)
	}
	check(&five, &five)
	// A role change immediately switches the quota to that level's setting.
	if _, err := p.pool.Exec(ctx, `UPDATE users SET role='LEVEL_2' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	usage, err := p.ATHubUsage(ctx, user)
	if err != nil || usage.Quota == nil || usage.Quota.TotalCallLimit != nil || usage.Quota.RemainingCalls != nil {
		t.Fatalf("LT2 quota=%+v error=%v", usage.Quota, err)
	}
}
