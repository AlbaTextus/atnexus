package athubaccess

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/google/uuid"
)

// submissionStore is the account store of a level whose changes may need
// review. What it sends to review is kept by queue, which stands in for the
// creator database and is attached to the pipeline separately.
type submissionStore struct {
	*accessStore
	store.ATHubPolicyStore
	policy store.ATHubRolePolicy
	queue  reviewQueue
}

func (s *submissionStore) ATHubPolicy(context.Context, string) (store.ATHubRolePolicy, error) {
	return s.policy, nil
}

// reviewQueue is the creator database as far as these tests reach it: it keeps
// the change that was last sent to review.
type reviewQueue struct {
	store.NovelHistoryStore
	store.ATHubReviewStore
	queued *store.ATHubReview
}

func (q *reviewQueue) QueueATHubReview(_ context.Context, item store.ATHubReview) (store.ATHubReview, error) {
	q.queued = &item
	return item, nil
}

// accountReviewQueue is an account store that could keep reviews itself, as
// the account database once did.
type accountReviewQueue struct {
	*submissionStore
	store.ATHubReviewStore
	asked bool
}

func (s *accountReviewQueue) QueueATHubReview(_ context.Context, item store.ATHubReview) (store.ATHubReview, error) {
	s.asked = true
	return item, nil
}

func TestAccountSubmissionAdmissionAndReview(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		beginErr error
		review   bool
		allowed  bool
		want     int
	}{
		{"direct upload", nil, false, true, 201},
		{"moderated upload", nil, true, true, 202},
		{"forbidden policy", nil, false, false, 403},
		{"permission changed during admission", store.ErrATHubPermissionDenied, false, true, 403},
		{"revoked session", store.ErrATHubAccessDenied, false, true, 401},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			base := &accessStore{beginErr: scenario.beginErr}
			st := &submissionStore{accessStore: base, policy: store.ATHubRolePolicy{
				ReviewRequired: scenario.review,
				Rules:          []store.ATHubAccessRule{{Operation: "upload", Allowed: scenario.allowed}},
			}}
			access, err := New(st, ":8003", nil)
			if err != nil {
				t.Fatal(err)
			}
			access.EnableCreatorStorage(&st.queue)
			dispatched := false
			access.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// A reviewed change is rehearsed first; only a real call counts.
				if athub.Rehearsing(r.Context()) {
					w.WriteHeader(200)
					return
				}
				dispatched = true
				body, _ := io.ReadAll(r.Body)
				if r.Method != "POST" || r.URL.Path != "/api/v2/upload" || r.URL.RawQuery != "" ||
					r.Header.Get("Authorization") != "" || string(body) != `{"novel":{"title":"测试小说"}}` {
					t.Fatal("submission was not constrained to the upload endpoint")
				}
				w.WriteHeader(201)
			}))
			user, session := uuid.New(), uuid.New()
			request := httptest.NewRequest("POST", "/api/v1/account/athub/submissions?user_id=spoofed", strings.NewReader(`{"novel":{"title":"测试小说"}}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer ath_untrusted")
			response := httptest.NewRecorder()
			access.SubmitUpload(response, request, user, session)
			if response.Code != scenario.want || dispatched != (scenario.want == 201) {
				t.Fatalf("status=%d body=%s dispatched=%v", response.Code, response.Body, dispatched)
			}
			if base.call.UserID == nil || *base.call.UserID != user || base.call.SessionID == nil ||
				*base.call.SessionID != session || base.call.KeyID != nil || !base.finished || base.status != scenario.want {
				t.Fatal("submission lost session ownership or audit completion")
			}
			if scenario.want == 202 {
				if queued := st.queue.queued; queued == nil || queued.UserID == nil || *queued.UserID != user || queued.Path != "/api/v2/upload" ||
					!json.Valid(queued.Payload) || queued.ID != base.call.ID {
					t.Fatal("submission did not enter the review queue")
				}
			}
		})
	}
}

// Reviews are kept by the creator database only. A pipeline without one
// refuses a change that needs review: it neither applies the change nor hands
// it to an account store that could keep it.
func TestReviewedChangeNeedsTheCreatorDatabase(t *testing.T) {
	base := &accessStore{}
	st := &accountReviewQueue{submissionStore: &submissionStore{accessStore: base, policy: store.ATHubRolePolicy{
		ReviewRequired: true,
		Rules:          []store.ATHubAccessRule{{Operation: "upload", Allowed: true}},
	}}}
	access, err := New(st, ":8003", nil)
	if err != nil {
		t.Fatal(err)
	}
	access.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("a change that could not be queued reached the content API")
	}))
	request := httptest.NewRequest("POST", "/api/v1/account/athub/submissions", strings.NewReader(`{"novel":{"title":"测试小说"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	access.SubmitUpload(response, request, uuid.New(), uuid.New())
	if response.Code != 503 || !strings.Contains(response.Body.String(), "reviews_unavailable") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if st.asked || st.queue.queued != nil {
		t.Fatal("the change was queued outside the creator database")
	}
	if !base.finished || base.status != 503 {
		t.Fatalf("refusal was not recorded: finished=%v status=%d", base.finished, base.status)
	}
}

func TestPublicRequestCannotImpersonateAccountSubmission(t *testing.T) {
	st := &accessStore{}
	access := accessFor(t, st)
	handler := access.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("untrusted request dispatched")
	}))
	request := httptest.NewRequest("POST", "/api/v2/upload", strings.NewReader(`{}`))
	request.Header.Set("X-AlbaTextus-Client", "account-center")
	request.Header.Set("X-User-ID", uuid.NewString())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 401 || !st.finished {
		t.Fatal("public upload bypassed authentication")
	}
}
