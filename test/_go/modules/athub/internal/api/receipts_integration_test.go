package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testReviewReceipts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	st := storage.NewPostgres(pool)
	handler, err := NewWithMiddleware(st, func(next http.Handler) http.Handler { return next }, nil)
	if err != nil {
		t.Fatal(err)
	}
	submit := func(id, body, path string) *httptest.ResponseRecorder {
		ctx := storage.WithMutationID(context.Background(), id)
		r := httptest.NewRequest("POST", path, strings.NewReader(body)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	id := "a6786c53-6c18-41cf-bdc6-a453b2a2fa10"
	first := submit(id, `{"name":"receipt-platform"}`, "/api/v2/platforms")
	second := submit(id, `{"name":"receipt-platform"}`, "/api/v2/platforms")
	if first.Code != 201 || second.Code != 201 || first.Body.String() != second.Body.String() {
		t.Fatalf("replay response changed: %d/%d %s/%s", first.Code, second.Code, first.Body, second.Body)
	}
	var platform struct {
		ID int32 `json:"id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &platform); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM "Platform" WHERE name='receipt-platform'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("replayed mutation count=%d err=%v", count, err)
	}
	results := make(chan *httptest.ResponseRecorder, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- submit("a6786c53-6c18-41cf-bdc6-a453b2a2fa11", `{"name":"concurrent-receipt"}`, "/api/v2/platforms")
		}()
	}
	workers.Wait()
	close(results)
	for result := range results {
		if result.Code != 201 {
			t.Fatalf("concurrent receipt: %d %s", result.Code, result.Body)
		}
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM "Platform" WHERE name='concurrent-receipt'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent mutations=%d err=%v", count, err)
	}
	// Once content committed the request has succeeded, even if finishing the
	// host audit failed: the caller gets the response, the replay returns the
	// same durable response without executing again, and the revision receipt
	// lets the host settle the audit it could not finish.
	var batches []string
	var outcomes []bool
	st.RecordChanges = func(_ context.Context, batch string, _ []storage.RevisionDraft) (func(context.Context, bool) error, error) {
		batches = append(batches, batch)
		return func(_ context.Context, committed bool) error {
			outcomes = append(outcomes, committed)
			return errors.New("host completion temporarily unavailable")
		}, nil
	}
	body := fmt.Sprintf(`{"platformId":%d,"title":"receipt-novel","status":"ONGOING"}`, platform.ID)
	committed := submit("a6786c53-6c18-41cf-bdc6-a453b2a2fa12", body, "/api/v2/novelinfo")
	retry := submit("a6786c53-6c18-41cf-bdc6-a453b2a2fa12", body, "/api/v2/novelinfo")
	if committed.Code != 201 || retry.Code != 201 || committed.Body.String() != retry.Body.String() {
		t.Fatalf("post-commit recovery: %d/%d %s/%s", committed.Code, retry.Code, committed.Body, retry.Body)
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM "Novel" WHERE title='receipt-novel'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("post-commit duplicated content=%d err=%v", count, err)
	}
	if len(batches) != 1 || len(outcomes) != 1 || !outcomes[0] {
		t.Fatalf("replay recorded again or commit reported as failed: batches=%v outcomes=%v", batches, outcomes)
	}
	if settled, err := st.CommittedRevisions(context.Background(), batches); err != nil || len(settled) != 1 || settled[0] != batches[0] {
		t.Fatalf("unfinished revision cannot be settled: %v err=%v", settled, err)
	}
	// Once the host has recorded the outcome it releases the receipt; forgetting
	// an already released receipt stays harmless.
	for range 2 {
		if err := st.DeleteMutationReceipt(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM "MutationReceipt" WHERE id=$1::uuid`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("released receipts=%d err=%v", count, err)
	}
}
