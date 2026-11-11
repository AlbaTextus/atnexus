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
