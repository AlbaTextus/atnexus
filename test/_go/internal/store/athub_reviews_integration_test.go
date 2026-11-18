package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestATHubReviewLifecycle(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	user, key := policyUser(t, p, "LEVEL_2")
	reviewer, _ := policyUser(t, p, "ADMIN")
	super, _ := policyUser(t, p, "SUPER_ADMIN")
	other, _ := policyUser(t, p, "USER")
	queue := func(method, path string) ATHubReview {
		call := policyCall(user, key, method, path)
		if err := p.BeginATHubCall(ctx, call); err != nil {
			t.Fatal(err)
		}
		contributor, _ := json.Marshal(map[string]string{"id": user.String(), "name": "提交者"})
		r, err := p.QueueATHubReview(ctx, ATHubReview{ID: call.ID, UserID: &user, Contributor: contributor, Operation: ATHubOperation(method, path), Method: method, Path: path, Payload: json.RawMessage(`{"name":"待审核内容"}`)})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	item := queue("POST", "/api/v2/tags")
	if item.Status != "pending" {
		t.Fatal("submission bypassed review")
	}
	if _, err := p.ListATHubReviews(ctx, user, true, "pending", 0); !errors.Is(err, ErrUserForbidden) {
		t.Fatal("LT2 accessed review queue", err)
	}
	if _, err := p.ATHubReview(ctx, other, item.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-account submission leaked", err)
	}
	list, err := p.ListATHubReviews(ctx, reviewer, true, "pending", 0)
	if err != nil || list.Total != 1 || len(list.Items) != 1 || list.Items[0].Payload != nil {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	var applied atomic.Int32
	execute := func(_ context.Context, r ATHubReview) (ATHubReviewResult, error) {
		applied.Add(1)
		if *r.UserID != user {
			t.Error("contributor changed")
		}
		return ATHubReviewResult{Status: 201, Body: json.RawMessage(`{"id":1}`)}, nil
	}
	if applied.Load() != 0 {
		t.Fatal("pending review executed")
	}
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, actor := range []uuid.UUID{reviewer, super} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := p.DecideATHubReview(ctx, actor, item.ID, "approve", "通过", execute)
			results <- err
		}()
	}
	workers.Wait()
	close(results)
	approved, conflicts := 0, 0
	for err := range results {
		if err == nil {
			approved++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if approved != 1 || conflicts != 1 || applied.Load() != 1 {
		t.Fatalf("approved=%d conflicts=%d executions=%d", approved, conflicts, applied.Load())
	}
	finished, err := p.ATHubReview(ctx, user, item.ID, false)
	if err != nil || finished.Status != "approved" || finished.Reviewer == nil {
		t.Fatal("review result not retained", err)
	}
	rejected := queue("POST", "/api/v2/tags")
	if result, err := p.DecideATHubReview(ctx, reviewer, rejected.ID, "reject", "内容不完整", execute); err != nil || result.Status != "rejected" || applied.Load() != 1 {
		t.Fatal("rejection applied content", err)
	}
	// A promotion must not let a submitter approve their own earlier submission.
	self := queue("POST", "/api/v2/tags")
	if _, err := p.pool.Exec(ctx, `UPDATE users SET role='ADMIN' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DecideATHubReview(ctx, user, self.ID, "approve", "", execute); !errors.Is(err, ErrUserForbidden) {
		t.Fatal("self approval accepted", err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE users SET role='LEVEL_2' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	// An application outage leaves the item pending so an idempotent retry is possible.
	if _, err := p.DecideATHubReview(ctx, reviewer, self.ID, "approve", "", func(context.Context, ATHubReview) (ATHubReviewResult, error) {
		return ATHubReviewResult{Status: 503}, nil
	}); err == nil {
		t.Fatal("outage marked approved")
	}
	pending, err := p.ATHubReview(ctx, user, self.ID, false)
	if err != nil || pending.Status != "pending" {
		t.Fatal("outage lost pending review", err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE users SET athub_disabled=true WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DecideATHubReview(ctx, reviewer, self.ID, "approve", "", execute); !errors.Is(err, ErrATHubPermissionDenied) {
		t.Fatal("disabled submitter retained approval", err)
	}
}
