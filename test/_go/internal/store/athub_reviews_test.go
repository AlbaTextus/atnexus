package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Review JSON is what the account and review pages receive. The baseline is
// bookkeeping between the content API and the review queue: it never appears
// there and is never accepted from there.
func TestATHubReviewJSONLeavesOutTheBaseline(t *testing.T) {
	author := uuid.New()
	review := ATHubReview{
		ID: uuid.New(), UserID: &author, Contributor: json.RawMessage(`{"name":"投稿者"}`),
		Operation: "edit", Method: "PUT", Path: "/api/v2/novelinfo", Query: "id=17",
		Payload: json.RawMessage(`{"title":"新标题"}`), Status: "pending",
		Baseline: json.RawMessage(`{"v":1,"novels":{"17":[{"before":{"title":"only-in-the-baseline"}}]}}`),
	}
	for name, value := range map[string]any{
		"review":  review,
		"pointer": &review,
		"list":    ATHubReviewList{Items: []ATHubReview{review}, Total: 1},
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if text := string(encoded); !strings.Contains(text, "新标题") || strings.Contains(strings.ToLower(text), "baseline") {
			t.Fatalf("%s: %s", name, text)
		}
	}
	var received ATHubReview
	if err := json.Unmarshal([]byte(`{"id":"`+review.ID.String()+`","baseline":{"v":1},"Baseline":{"v":1}}`), &received); err != nil || received.ID != review.ID || received.Baseline != nil {
		t.Fatalf("received=%+v err=%v", received, err)
	}
}
