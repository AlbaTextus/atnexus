package athubaccess

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

func TestIllustrationDeleteUsesSessionPermissionsAndReview(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		review  bool
		allowed bool
		want    int
	}{
		{"direct", false, true, 204},
		{"moderated", true, true, 202},
		{"denied", false, false, 403},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			base := &accessStore{}
			st := &submissionStore{accessStore: base, policy: store.ATHubRolePolicy{
				ReviewRequired: scenario.review,
				Rules:          []store.ATHubAccessRule{{Operation: "delete", Allowed: scenario.allowed}},
			}}
			access, err := New(st, ":8003", nil)
			if err != nil {
				t.Fatal(err)
			}
			dispatched := false
			access.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched = true
				if r.Method != "DELETE" || r.URL.Path != "/api/v2/illustrations" ||
					r.URL.Query().Get("novelId") != "17" || r.URL.Query().Get("id") != "23" ||
					r.Header.Get("Authorization") != "" {
					t.Fatal("delete escaped the constrained illustration endpoint")
				}
				w.WriteHeader(204)
			}))
			user, session := uuid.New(), uuid.New()
			request := httptest.NewRequest("DELETE", "/untrusted?novelId=99&id=98", nil)
			request.Header.Set("Authorization", "Bearer untrusted")
			response := httptest.NewRecorder()
			access.DeleteIllustration(response, request, user, session, 17, 23)
			if response.Code != scenario.want || dispatched != (scenario.want == 204) {
				t.Fatalf("status=%d body=%s dispatched=%v", response.Code, response.Body, dispatched)
			}
			if base.call.UserID == nil || *base.call.UserID != user || base.call.SessionID == nil || *base.call.SessionID != session || !base.finished {
				t.Fatal("delete lost account session or audit")
			}
			if scenario.review && (st.queued == nil || st.queued.Method != "DELETE" ||
				st.queued.Path != "/api/v2/illustrations" || st.queued.Query != "id=23&novelId=17") {
				t.Fatal("delete lost parent/image identity in review")
			}
		})
	}
}

func TestIllustrationLayoutUsesEditPermissionAndReview(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		review, allowed bool
		want            int
	}{
		{"direct", false, true, 200},
		{"moderated", true, true, 202},
		{"denied", false, false, 403},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			base := &accessStore{}
			st := &submissionStore{accessStore: base, policy: store.ATHubRolePolicy{
				ReviewRequired: scenario.review,
				Rules:          []store.ATHubAccessRule{{Operation: "edit", Allowed: scenario.allowed}},
			}}
			access, err := New(st, ":8003", nil)
			if err != nil {
				t.Fatal(err)
			}
			called := false
			access.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.Method != "PUT" || r.URL.Path != "/api/v2/illustration-layout" || r.URL.Query().Get("novelId") != "17" || r.Header.Get("Authorization") != "" {
					t.Fatal("layout escaped constrained endpoint")
				}
				w.WriteHeader(200)
			}))
			r := httptest.NewRequest("PUT", "/untrusted?novelId=99", strings.NewReader(`{"novelId":17,"revision":0,"blocks":[]}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer untrusted")
			w := httptest.NewRecorder()
			user, session := uuid.New(), uuid.New()
			access.SubmitIllustrationLayout(w, r, user, session, 17)
			if w.Code != scenario.want || called != (scenario.want == 200) {
				t.Fatalf("status=%d body=%s called=%v", w.Code, w.Body, called)
			}
			if base.call.UserID == nil || *base.call.UserID != user || base.call.SessionID == nil || *base.call.SessionID != session || !base.finished {
				t.Fatal("layout lost account session or audit")
			}
			if scenario.review && (st.queued == nil || st.queued.Query != "novelId=17" || st.queued.Method != "PUT" || st.queued.Path != "/api/v2/illustration-layout") {
				t.Fatal("layout did not enter review queue")
			}
		})
	}
}
