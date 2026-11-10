package storage

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabaseErrors(t *testing.T) {
	unknown := errors.New("connection failed")
	for _, tc := range []struct{ input, expected error }{
		{nil, nil},
		{pgx.ErrNoRows, ErrNotFound},
		{fmt.Errorf("wrapped: %w", pgx.ErrNoRows), ErrNotFound},
		{&pgconn.PgError{Code: "23503"}, ErrInvalidReference},
		{&pgconn.PgError{Code: "23505"}, ErrConflict},
		{unknown, unknown},
	} {
		if got := databaseError(tc.input); !errors.Is(got, tc.expected) {
			t.Errorf("%v: got %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestDeleted(t *testing.T) {
	if err := deleted(pgconn.NewCommandTag("DELETE 1"), nil); err != nil {
		t.Fatal(err)
	}
	if err := deleted(pgconn.NewCommandTag("DELETE 0"), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if err := deleted(pgconn.CommandTag{}, &pgconn.PgError{Code: "23503"}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("got %v", err)
	}
}
