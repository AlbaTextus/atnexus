package storage

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type mutationIDKey struct{}
type receiptKey struct{}
type MutationReceipt struct {
	ID     string      `json:"-"`
	Header http.Header `json:"header"`
	Status int         `json:"status"`
	Body   []byte      `json:"body"`
}

type rehearsalKey struct{}

// Rehearsal receives what a rehearsed change turned out to alter.
type Rehearsal struct{ baseline []byte }

// Baseline describes the content a rehearsed change alters, as it was when the
// rehearsal ran. It is nil unless the rehearsal succeeded. WithBaseline takes
// it back when the same change is applied later.
func (r *Rehearsal) Baseline() []byte { return r.baseline }

// WithRehearsal makes Mutate run a change in full and then roll it back. The
// caller learns how the change would be answered right now and what it would
// alter, while nothing is stored, recorded as a revision or remembered as a
// receipt.
func WithRehearsal(ctx context.Context) (context.Context, *Rehearsal) {
	rehearsal := &Rehearsal{}
	return context.WithValue(ctx, rehearsalKey{}, rehearsal), rehearsal
}

// Rehearsing reports whether ctx belongs to a rehearsal.
func Rehearsing(ctx context.Context) bool {
	rehearsal, _ := ctx.Value(rehearsalKey{}).(*Rehearsal)
	return rehearsal != nil
}

func WithMutationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, mutationIDKey{}, id)
}
func NewMutationReceipt(ctx context.Context) (context.Context, *MutationReceipt) {
	id, _ := ctx.Value(mutationIDKey{}).(string)
	if id == "" || Rehearsing(ctx) {
		return ctx, nil
	}
	receipt := &MutationReceipt{ID: id}
	return context.WithValue(ctx, receiptKey{}, receipt), receipt
}

// DeleteMutationReceipt forgets a replay record. The host calls it only after it
// has durably recorded the outcome, when the same ID can never be applied again.
// Deleting an absent receipt is not an error.
func (p *Postgres) DeleteMutationReceipt(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM "MutationReceipt" WHERE id=$1::uuid`, id)
	return err
}

// CommittedRevisions returns those of the given revision batches whose content
// transaction committed. A batch that is absent never changed any content.
func (p *Postgres) CommittedRevisions(ctx context.Context, batches []string) ([]string, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text FROM "RevisionReceipt" WHERE id=ANY($1::uuid[])`, batches)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// DeleteRevisionReceipts drops receipts written before the given time. The
// host calls it only after settling every revision that old.
func (p *Postgres) DeleteRevisionReceipts(ctx context.Context, before time.Time) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM "RevisionReceipt" WHERE "createdAt"<$1`, before)
	return err
}

// A change that starts to point at a stored image and the removal of that
// image take the same lock, so neither acts on an outdated view of the other.
const imageLock = `SELECT pg_advisory_xact_lock(hashtextextended($1,118))`

// HoldImage makes the change in progress wait for a removal of the image at
// location that is under way, and makes later removals wait until the change
// has ended. The caller then checks that the image is still stored.
func (p *Postgres) HoldImage(ctx context.Context, location string) error {
	_, err := p.db(ctx).Exec(ctx, imageLock, location)
	return err
}

// ReleaseImage calls remove unless a novel cover or illustration points to
// location, with or without a query or fragment after it. A change that would
// start to point at it waits until remove has returned, and then finds the
// image gone.
func (p *Postgres) ReleaseImage(ctx context.Context, location string, remove func(context.Context) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err := tx.Exec(ctx, imageLock, location); err != nil {
		return err
	}
	var used bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM "Novel" WHERE starts_with("coverUrl",$1))
		OR EXISTS(SELECT 1 FROM "Illustration" WHERE starts_with(url,$1))`, location).Scan(&used); err != nil || used {
		return err
	}
	return remove(ctx)
}
