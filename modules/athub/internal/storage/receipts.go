package storage

import (
	"context"
	"net/http"
)

type mutationIDKey struct{}
type receiptKey struct{}
type MutationReceipt struct {
	ID     string      `json:"-"`
	Header http.Header `json:"header"`
	Status int         `json:"status"`
	Body   []byte      `json:"body"`
}

func WithMutationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, mutationIDKey{}, id)
}
func NewMutationReceipt(ctx context.Context) (context.Context, *MutationReceipt) {
	id, _ := ctx.Value(mutationIDKey{}).(string)
	if id == "" {
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
