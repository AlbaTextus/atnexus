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

// WithRehearsal makes Mutate run a change in full and then roll it back. The
// caller learns how the change would be answered right now, while nothing is
// stored, recorded as a revision or remembered as a receipt.
func WithRehearsal(ctx context.Context) context.Context {
	return context.WithValue(ctx, rehearsalKey{}, true)
}

// Rehearsing reports whether ctx belongs to a rehearsal.
func Rehearsing(ctx context.Context) bool {
	rehearsing, _ := ctx.Value(rehearsalKey{}).(bool)
	return rehearsing
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
