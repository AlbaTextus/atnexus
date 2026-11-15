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
	"github.com/google/uuid"
)

type submissionStore struct {
	*accessStore
	store.ATHubPolicyStore
	store.ATHubReviewStore
	policy store.ATHubRolePolicy
	queued *store.ATHubReview
}

func (s *submissionStore) ATHubPolicy(context.Context, string) (store.ATHubRolePolicy, error) {
	return s.policy, nil
}

func (s *submissionStore) QueueATHubReview(_ context.Context, item store.ATHubReview) (store.ATHubReview, error) {
	s.queued = &item
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
			dispatched := false
			access.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				if st.queued == nil || st.queued.UserID == nil || *st.queued.UserID != user || st.queued.Path != "/api/v2/upload" ||
					!json.Valid(st.queued.Payload) || st.queued.ID != base.call.ID {
					t.Fatal("submission did not enter the existing review queue")
				}
			}
		})
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
