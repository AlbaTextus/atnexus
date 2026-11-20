package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

// Opt-in. Reviews are kept in the creator database and every decision is
// checked against the accounts as they are when it is made. One review is
// decided once however many reviewers act on it at the same moment, and only an
// approval applies its change.
func TestReviewDecisionsAreSerializedAndChecked(t *testing.T) {
	accounts, creator, source, _ := splitFixture(t)
	ctx := context.Background()
	account := func(role string) uuid.UUID {
		t.Helper()
		user, err := accounts.UpsertUser(ctx, store.UserSeed{Email: "decision-" + strings.ToLower(role) + "@example.test", PasswordHash: "unused", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		return user.ID
	}
	author, reviewer, super := account("LEVEL_2"), account("ADMIN"), account("SUPER_ADMIN")
	key, err := accounts.CreateATHubKey(ctx, store.ATHubKey{ID: uuid.New(), UserID: author, Name: "test", TokenHash: security.HashToken(uuid.NewString()), Prefix: "ath_test"})
	if err != nil {
		t.Fatal(err)
	}
	queue := func() store.ATHubReview {
		t.Helper()
		call := store.ATHubCall{ID: uuid.New(), RequestID: uuid.New(), UserID: &author, KeyID: &key.ID, Method: "POST", Path: "/api/v2/tags", Authenticated: true}
		if err := accounts.BeginATHubCall(ctx, call); err != nil {
			t.Fatal(err)
		}
		contributor, _ := json.Marshal(map[string]string{"id": author.String(), "name": "提交者"})
		item, err := creator.QueueATHubReview(ctx, store.ATHubReview{
			ID: call.ID, UserID: &author, Contributor: contributor,
			Operation: store.ATHubOperation(call.Method, call.Path), Method: call.Method, Path: call.Path,
			Payload: json.RawMessage(`{"name":"待审核内容"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	var applied atomic.Int32
	execute := func(_ context.Context, r store.ATHubReview) (store.ATHubReviewResult, error) {
		applied.Add(1)
		if r.UserID == nil || *r.UserID != author {
			t.Error("the change was applied for somebody other than its submitter")
		}
		return store.ATHubReviewResult{Status: 201, Body: json.RawMessage(`{"id":1}`)}, nil
	}

	item := queue()
	if item.Status != "pending" || applied.Load() != 0 {
		t.Fatalf("a submission bypassed review: status=%q applied=%d", item.Status, applied.Load())
	}
	// The queue lists a review without the change it carries.
	list, err := creator.ListATHubReviews(ctx, reviewer, true, "pending", 0)
	if err != nil || list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != item.ID || list.Items[0].Payload != nil {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	// Two reviewers approve at once: one decision stands and the change is
	// applied once.
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, actor := range []uuid.UUID{reviewer, super} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := creator.DecideATHubReview(ctx, actor, item.ID, "approve", "通过", execute)
			results <- err
		}()
	}
	workers.Wait()
	close(results)
	approved, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			approved++
		case errors.Is(err, store.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if approved != 1 || conflicts != 1 || applied.Load() != 1 {
		t.Fatalf("approved=%d conflicts=%d executions=%d", approved, conflicts, applied.Load())
	}
	finished, err := creator.ATHubReview(ctx, author, item.ID, false)
	if err != nil || finished.Status != "approved" || finished.Reviewer == nil || finished.ResultStatus == nil || *finished.ResultStatus != 201 {
		t.Fatalf("the decision was not kept with the review: %+v err=%v", finished, err)
	}

	rejected, err := creator.DecideATHubReview(ctx, reviewer, queue().ID, "reject", "内容不完整", execute)
	if err != nil || rejected.Status != "rejected" || rejected.Note != "内容不完整" || applied.Load() != 1 {
		t.Fatalf("a rejection applied content: review=%+v err=%v executions=%d", rejected, err, applied.Load())
	}

	// A promotion must not let a submitter approve their own earlier submission.
	own := queue()
	if _, err := source.Exec(ctx, `UPDATE users SET role='ADMIN' WHERE id=$1`, author); err != nil {
		t.Fatal(err)
	}
	if _, err := creator.DecideATHubReview(ctx, author, own.ID, "approve", "", execute); !errors.Is(err, store.ErrUserForbidden) {
		t.Fatal("a submitter approved their own submission", err)
	}
	if _, err := source.Exec(ctx, `UPDATE users SET role='LEVEL_2' WHERE id=$1`, author); err != nil {
		t.Fatal(err)
	}
	// A submitter barred from the library since has nothing approved for them.
	if _, err := source.Exec(ctx, `UPDATE users SET athub_disabled=true WHERE id=$1`, author); err != nil {
		t.Fatal(err)
	}
	if _, err := creator.DecideATHubReview(ctx, reviewer, own.ID, "approve", "", execute); !errors.Is(err, store.ErrATHubPermissionDenied) {
		t.Fatal("a barred submitter kept an approval", err)
	}
	pending, err := creator.ATHubReview(ctx, author, own.ID, false)
	if err != nil || pending.Status != "pending" || applied.Load() != 1 {
		t.Fatalf("a refused decision changed the review: %+v err=%v executions=%d", pending, err, applied.Load())
	}
}
