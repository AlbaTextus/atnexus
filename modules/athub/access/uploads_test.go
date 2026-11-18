package athubaccess

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/google/uuid"
)

var uploadKey = "ath_" + strings.Repeat("a", 43)

func TestKeyCoverUploadIsAdmittedBeforeTheFileIsStored(t *testing.T) {
	for _, scenario := range []struct {
		name                 string
		auth                 string
		active               bool
		resolveErr, beginErr error
		want                 int
	}{
		{"valid key", "Bearer " + uploadKey, true, nil, nil, 200},
		{"unknown key", "Bearer " + uploadKey, false, store.ErrNotFound, nil, 401},
		{"revoked expired or disabled", "Bearer " + uploadKey, false, nil, nil, 401},
		{"not a personal key", "Bearer " + strings.Repeat("t", 64), true, nil, nil, 401},
		{"covers forbidden for the level", "Bearer " + uploadKey, true, nil, store.ErrATHubPermissionDenied, 403},
		{"total call limit reached", "Bearer " + uploadKey, true, nil, store.ErrATHubCallLimit, 429},
		{"disabled between lookup and admission", "Bearer " + uploadKey, true, nil, store.ErrATHubAccessDenied, 401},
		{"key lookup unavailable", "Bearer " + uploadKey, true, errors.New("offline"), nil, 503},
		{"recording unavailable", "Bearer " + uploadKey, true, nil, errors.New("offline"), 503},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			base := &accessStore{
				identity:   store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: scenario.active},
				resolveErr: scenario.resolveErr, beginErr: scenario.beginErr,
			}
			// A cover is stored directly even for a level whose content changes are reviewed.
			st := &submissionStore{accessStore: base, policy: store.ATHubRolePolicy{
				ReviewRequired: true,
				Rules:          []store.ATHubAccessRule{{Operation: "upload", Allowed: true}},
			}}
			access, err := New(st, ":8003", nil)
			if err != nil {
				t.Fatal(err)
			}
			// A signed-in browser must not stand in for a key that was refused.
			access.EnableSessions(sessionVerifier{identity: auth.Identity{UserID: uuid.New(), SessionID: uuid.New()}})
			access.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("cover upload reached the content API")
			}))
			saved := false
			request := httptest.NewRequest("POST", "/api/v4/account/athub/cover?novelId=9", strings.NewReader("image"))
			request.Header.Set("Authorization", scenario.auth)
			request.Header.Set("X-AlbaTextus-Client", "account-center")
			response := httptest.NewRecorder()
			access.UploadCoverWithKey(response, request, func(w http.ResponseWriter, r *http.Request, user uuid.UUID) {
				saved = true
				if base.call.ID == uuid.Nil || !base.call.Authenticated {
					t.Fatal("cover stored before the call was admitted")
				}
				body, _ := io.ReadAll(r.Body)
				if user != base.identity.UserID || string(body) != "image" {
					t.Fatal("cover lost its owner or its file")
				}
				w.WriteHeader(200)
			})
			if response.Code != scenario.want || saved != (scenario.want == 200) {
				t.Fatalf("status=%d body=%s saved=%v", response.Code, response.Body, saved)
			}
			if st.queued != nil {
				t.Fatal("cover upload entered the review queue")
			}
			if scenario.want == 503 {
				if base.finished {
					t.Fatal("unrecorded call was completed")
				}
				return
			}
			if !base.finished || base.status != scenario.want {
				t.Fatalf("completion missing: finished=%v status=%d", base.finished, base.status)
			}
			if base.call.Method != "POST" || base.call.Path != store.ATHubCoverPath || base.call.Port != 8003 || base.call.SessionID != nil {
				t.Fatalf("unexpected audit labels: %+v", base.call)
			}
			if scenario.want == 200 && (base.call.KeyID == nil || *base.call.KeyID != base.identity.KeyID ||
				base.call.UserID == nil || *base.call.UserID != base.identity.UserID) {
				t.Fatalf("call was not attributed to the key: %+v", base.call)
			}
			if scenario.want == 401 && response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing bearer challenge")
			}
		})
	}
}

func TestKeyIllustrationUploadFollowsPermissionsAndReview(t *testing.T) {
	const stored = `{"url":"https://images.example/a.jpg","sha256":"00","width":4,"height":3}`
	for _, scenario := range []struct {
		name            string
		beginErr        error
		review, allowed bool
		refused         bool
		want            int
		saved           bool
	}{
		{"direct", nil, false, true, false, 201, true},
		{"moderated", nil, true, true, false, 202, true},
		{"permission changed after admission", nil, false, false, false, 403, true},
		{"forbidden for the level", store.ErrATHubPermissionDenied, false, true, false, 403, false},
		{"total call limit reached", store.ErrATHubCallLimit, false, true, false, 429, false},
		{"revoked key", store.ErrATHubAccessDenied, false, true, false, 401, false},
		{"image refused", nil, false, true, true, 400, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			base := &accessStore{
				identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: true},
				beginErr: scenario.beginErr,
			}
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
				if r.Method != "POST" || r.URL.Path != "/api/v2/illustrations" || r.URL.RawQuery != "novelId=17" ||
					r.Header.Get("Content-Type") != "application/json" || r.ContentLength != int64(len(stored)) || string(body) != stored {
					t.Fatal("upload escaped the constrained illustration endpoint")
				}
				w.WriteHeader(201)
			}))
			saved := false
			request := httptest.NewRequest("POST", "/api/v4/account/athub/novels/99/illustrations?novelId=98", strings.NewReader("image"))
			request.Header.Set("Authorization", "Bearer "+uploadKey)
			request.Header.Set("Content-Type", "multipart/form-data; boundary=x")
			response := httptest.NewRecorder()
			access.UploadIllustrationWithKey(response, request, func(w http.ResponseWriter, r *http.Request) (int32, []byte, bool) {
				saved = true
				if base.call.ID == uuid.Nil {
					t.Fatal("illustration stored before the call was admitted")
				}
				if body, _ := io.ReadAll(r.Body); string(body) != "image" {
					t.Fatal("illustration lost its file")
				}
				if scenario.refused {
					w.WriteHeader(400)
					return 0, nil, false
				}
				return 17, []byte(stored), true
			})
			if response.Code != scenario.want || saved != scenario.saved || dispatched != (scenario.want == 201) {
				t.Fatalf("status=%d body=%s saved=%v dispatched=%v", response.Code, response.Body, saved, dispatched)
			}
			if !base.finished || base.status != scenario.want {
				t.Fatalf("completion missing: finished=%v status=%d", base.finished, base.status)
			}
			if base.call.Method != "POST" || base.call.Path != "/api/v2/illustrations" || base.call.SessionID != nil ||
				base.call.KeyID == nil || *base.call.KeyID != base.identity.KeyID {
				t.Fatalf("call was not recorded as a key illustration upload: %+v", base.call)
			}
			if (st.queued != nil) != (scenario.want == 202) {
				t.Fatalf("review queue=%+v", st.queued)
			}
			if scenario.want == 202 && (st.queued.ID != base.call.ID || st.queued.UserID == nil || *st.queued.UserID != base.identity.UserID ||
				st.queued.Operation != "upload" || st.queued.Method != "POST" || st.queued.Path != "/api/v2/illustrations" ||
				st.queued.Query != "novelId=17" || string(st.queued.Payload) != stored) {
				t.Fatalf("illustration lost its novel or image in review: %+v", st.queued)
			}
		})
	}
}

func TestKeyIllustrationUploadNeedsTheContentAPI(t *testing.T) {
	st := &accessStore{identity: store.ATHubIdentity{KeyID: uuid.New(), UserID: uuid.New(), Active: true}}
	request := httptest.NewRequest("POST", "/api/v4/account/athub/novels/17/illustrations", strings.NewReader("image"))
	request.Header.Set("Authorization", "Bearer "+uploadKey)
	response := httptest.NewRecorder()
	accessFor(t, st).UploadIllustrationWithKey(response, request, func(http.ResponseWriter, *http.Request) (int32, []byte, bool) {
		t.Fatal("illustration stored without a content API to record it")
		return 0, nil, false
	})
	if response.Code != 503 || st.call.ID != uuid.Nil {
		t.Fatalf("status=%d call=%+v", response.Code, st.call)
	}
}
