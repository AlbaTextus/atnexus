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

func TestAccountNovelEditAdmissionAndReview(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		beginErr error
		review   bool
		base     bool
		override *bool
		want     int
	}{
		{name: "direct edit", base: true, want: 200},
		{name: "moderated edit", base: true, review: true, want: 202},
		{name: "forbidden policy", want: 403},
		{name: "endpoint override denies base permission", base: true, override: boolPointer(false), want: 403},
		{name: "endpoint override grants permission", override: boolPointer(true), review: true, want: 202},
		{name: "permission changed during admission", base: true, beginErr: store.ErrATHubPermissionDenied, want: 403},
		{name: "revoked session", base: true, beginErr: store.ErrATHubAccessDenied, want: 401},
		{name: "call limit", base: true, beginErr: store.ErrATHubCallLimit, want: 429},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			base := &accessStore{beginErr: scenario.beginErr}
			st := &submissionStore{accessStore: base, policy: store.ATHubRolePolicy{
				ReviewRequired: scenario.review,
				Rules:          []store.ATHubAccessRule{{Operation: "edit", Allowed: scenario.base}},
			}}
			if scenario.override != nil {
				st.policy.Rules = append(st.policy.Rules, store.ATHubAccessRule{
					Operation: "edit", Method: "PUT", Path: "/api/v2/novelinfo", Allowed: *scenario.override,
				})
			}
			access, err := New(st, ":8003", nil)
			if err != nil {
				t.Fatal(err)
			}
			dispatched := false
			payload := `{"platformId":1,"title":"修改后的小说","status":"COMPLETED","tagIds":[2]}`
			access.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// A reviewed change is rehearsed first; only a real call counts.
				if athub.Rehearsing(r.Context()) {
					w.WriteHeader(200)
					return
				}
				dispatched = true
				body, _ := io.ReadAll(r.Body)
				if r.Method != "PUT" || r.URL.Path != "/api/v2/novelinfo" || r.URL.RawQuery != "id=17" ||
					r.Header.Get("Authorization") != "" || string(body) != payload {
					t.Fatal("edit escaped the selected novel endpoint")
				}
				w.Write([]byte(`{"id":17}`))
			}))
			user, session := uuid.New(), uuid.New()
			request := httptest.NewRequest("PUT", "/api/v1/account/athub/novels/17?id=99&path=/api/v2/content", strings.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer ath_untrusted")
			response := httptest.NewRecorder()
			access.SubmitNovelEdit(response, request, user, session, 17)
			if response.Code != scenario.want || dispatched != (scenario.want == 200) {
				t.Fatalf("status=%d body=%s dispatched=%v", response.Code, response.Body, dispatched)
			}
			if base.call.UserID == nil || *base.call.UserID != user || base.call.SessionID == nil ||
				*base.call.SessionID != session || base.call.KeyID != nil || !base.finished || base.status != scenario.want {
				t.Fatal("edit lost session ownership or audit completion")
			}
			if scenario.want == 202 {
				item := st.queued
				if item == nil || item.Operation != "edit" || item.Method != "PUT" || item.Query != "id=17" ||
					item.Path != "/api/v2/novelinfo" || item.UserID == nil || *item.UserID != user ||
					!json.Valid(item.Payload) || string(item.Payload) != payload || item.ID != base.call.ID {
					t.Fatal("edit did not enter the review queue with the original payload")
				}
				// Approval replays this exact edit using the recorded contributor.
				result, err := access.ApplyReview(context.Background(), *item)
				if err != nil || result.Status != 200 || !dispatched {
					t.Fatalf("approved edit not applied: result=%+v err=%v", result, err)
				}
			} else if st.queued != nil {
				t.Fatal("unexpected queued edit")
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func TestPublicNovelEditCannotImpersonateAccount(t *testing.T) {
	st := &accessStore{}
	access := accessFor(t, st)
	handler := access.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("untrusted edit dispatched")
	}))
	request := httptest.NewRequest("PUT", "/api/v2/novelinfo?id=17", strings.NewReader(`{}`))
	request.Header.Set("X-AlbaTextus-Client", "account-center")
	request.Header.Set("X-User-ID", uuid.NewString())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 401 || !st.finished {
		t.Fatal("public edit bypassed authentication")
	}
}
