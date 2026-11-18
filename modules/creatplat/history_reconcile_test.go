package creatplat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

type pendingRevisions struct {
	created  map[uuid.UUID]time.Time
	finished map[uuid.UUID]bool
}

func (p *pendingRevisions) StaleRevisionBatches(_ context.Context, before time.Time, _ int) ([]uuid.UUID, error) {
	var stale []uuid.UUID
	for batch, at := range p.created {
		if _, done := p.finished[batch]; !done && at.Before(before) {
			stale = append(stale, batch)
		}
	}
	return stale, nil
}

func (p *pendingRevisions) FinishNovelRevisions(_ context.Context, batch uuid.UUID, committed bool) error {
	if _, done := p.finished[batch]; done {
		return store.ErrNotFound
	}
	p.finished[batch] = committed
	return nil
}

type contentReceipts struct {
	committed []string
	failure   error
	pruned    time.Time
}

func (c *contentReceipts) CommittedRevisions(context.Context, []string) ([]string, error) {
	return c.committed, c.failure
}

func (c *contentReceipts) PruneRevisionReceipts(_ context.Context, before time.Time) error {
	c.pruned = before
	return nil
}

func TestReconcileSettlesStaleRevisionsFromContentReceipts(t *testing.T) {
	now := time.Now()
	committed, abandoned, inFlight := uuid.New(), uuid.New(), uuid.New()
	revisions := &pendingRevisions{finished: map[uuid.UUID]bool{}, created: map[uuid.UUID]time.Time{
		committed: now.Add(-time.Hour), abandoned: now.Add(-time.Hour), inFlight: now.Add(-time.Minute),
	}}
	content := &contentReceipts{committed: []string{committed.String()}}
	settled, err := NewHistory(revisions, nil).Reconcile(context.Background(), content, now)
	if err != nil || settled != 2 {
		t.Fatalf("settled=%d err=%v", settled, err)
	}
	if done, ok := revisions.finished[committed]; !ok || !done {
		t.Fatal("a batch with a content receipt must be completed")
	}
	if done, ok := revisions.finished[abandoned]; !ok || done {
		t.Fatal("a batch without a content receipt must be failed")
	}
	if _, ok := revisions.finished[inFlight]; ok {
		t.Fatal("a batch that may still be in flight must be left alone")
	}
	if !content.pruned.Equal(now.Add(-revisionReceiptLife)) {
		t.Fatalf("receipts pruned before %v", content.pruned)
	}
}

func TestReconcileGuessesNothingWhenContentIsUnavailable(t *testing.T) {
	now := time.Now()
	batch := uuid.New()
	revisions := &pendingRevisions{finished: map[uuid.UUID]bool{}, created: map[uuid.UUID]time.Time{batch: now.Add(-time.Hour)}}
	content := &contentReceipts{failure: errors.New("connection refused")}
	if _, err := NewHistory(revisions, nil).Reconcile(context.Background(), content, now); err == nil {
		t.Fatal("an unreachable content database must be reported")
	}
	if len(revisions.finished) != 0 || !content.pruned.IsZero() {
		t.Fatal("nothing may be settled or pruned without an answer from the content database")
	}
}
