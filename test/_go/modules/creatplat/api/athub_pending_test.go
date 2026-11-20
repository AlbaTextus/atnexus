package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/google/uuid"
)

// pendingStore is a creator database that lists what it is given and remembers
// what it was asked.
type pendingStore struct {
	store.ATHubReviewStore
	list   store.ATHubPendingChangeList
	err    error
	calls  int
	actor  uuid.UUID
	novel  int32
	offset int
}

func (s *pendingStore) PendingATHubChanges(_ context.Context, actor uuid.UUID, novelID int32, offset int) (store.ATHubPendingChangeList, error) {
	s.calls++
	s.actor, s.novel, s.offset = actor, novelID, offset
	return s.list, s.err
}

// contentChanges reads a baseline the way the host has the content library
// read it.
func contentChanges(baseline []byte, novelID int32) (any, error) {
	return athub.SubmittedChanges(baseline, novelID)
}

// pendingAPI is an API whose creator database is the returned store.
func pendingAPI(t *testing.T) (*API, *keyStore, *pendingStore) {
	t.Helper()
	a, keys := keyAPI(t)
	pending := &pendingStore{list: store.ATHubPendingChangeList{Items: []store.ATHubPendingChange{}}}
	a.reviews = pending
	a.SetATHubChangeReader(contentChanges)
	return a, keys, pending
}

// readPending asks for a novel's pending changes through the router, so the
// route itself is part of what is tested.
func readPending(a *API, novel, query string, alter func(*http.Request)) *httptest.ResponseRecorder {
	request := keyRequest("GET", "")
	request.URL.Path = "/api/v4/account/athub/novels/" + novel + "/pending-changes"
	request.URL.RawQuery = query
	if alter != nil {
		alter(request)
	}
	response := httptest.NewRecorder()
	a.Router().ServeHTTP(response, request)
	return response
}

func errorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("%v: %s", err, response.Body)
	}
	return body.Error
}

// Each submission is served with when it was made and what it does to the
// novel, in the order the store lists them. Nothing tells who made it, what the
// altered fields held before, or anything else the baseline keeps.
func TestPendingChangesServeWhatEachSubmissionAlters(t *testing.T) {
	a, keys, pending := pendingAPI(t)
	first, second := uuid.New(), uuid.New()
	earlier := time.Date(2026, 11, 18, 9, 30, 0, 123456000, time.UTC)
	later := earlier.Add(26 * time.Hour)
	pending.list = store.ATHubPendingChangeList{Total: 7, Items: []store.ATHubPendingChange{
		{ID: first, CreatedAt: earlier, Baseline: json.RawMessage(`{"v":1,"novels":{"17":[
			{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"只在基线里的旧标题","tagIds":[1,2]},"after":{"title":"新标题","tagIds":[2,3],"tagNames":["奇幻","冒险"]}},
			{"key":"Illustration:abc","action":"INSERT","after":{"novelId":17,"sha256":"abc","url":"https://img.example.test/a.webp","width":800,"height":600}},
			{"key":"Illustration:def","action":"DELETE","before":{"id":41,"novelId":17,"sha256":"def","url":"https://img.example.test/removed.webp"}}
		]}}`)},
		{ID: second, CreatedAt: later, Baseline: json.RawMessage(`{"v":1,"novels":{"17":[
			{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"只在基线里的旧标题"}}
		]}}`)},
	}}
	response := readPending(a, "17", "offset=50", func(r *http.Request) {
		// A read is not a write: it needs the session and nothing else.
		r.Header.Del("X-AlbaTextus-Client")
	})
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if pending.calls != 1 || pending.actor != keys.user.ID || pending.novel != 17 || pending.offset != 50 {
		t.Fatalf("the store was asked %d times for actor=%s novel=%d offset=%d", pending.calls, pending.actor, pending.novel, pending.offset)
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("Cache-Control=%q", cache)
	}
	var got any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, response.Body)
	}
	want := map[string]any{"total": float64(7), "items": []any{
		map[string]any{"id": first.String(), "created_at": "2026-11-18T09:30:00.123456Z", "changes": []any{
			map[string]any{"entity": "Novel", "action": "UPDATE", "id": float64(17), "fields": []any{"tagIds", "title"},
				"after": map[string]any{"title": "新标题", "tagIds": []any{float64(2), float64(3)}, "tagNames": []any{"奇幻", "冒险"}}},
			map[string]any{"entity": "Illustration", "action": "INSERT",
				"after": map[string]any{"novelId": float64(17), "sha256": "abc", "url": "https://img.example.test/a.webp", "width": float64(800), "height": float64(600)}},
			map[string]any{"entity": "Illustration", "action": "DELETE", "id": float64(41)},
		}},
		// Queued before later values were kept: the fields are named, and what
		// they become is unknown.
		map[string]any{"id": second.String(), "created_at": "2026-11-19T11:30:00.123456Z", "changes": []any{
			map[string]any{"entity": "Novel", "action": "UPDATE", "id": float64(17), "fields": []any{"title"}},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("served %s", response.Body)
	}
	body := response.Body.String()
	for _, kept := range []string{"baseline", "before", "只在基线里的旧标题", "removed.webp", "contributor", "user_id", keys.user.ID.String()} {
		if strings.Contains(body, kept) {
			t.Fatalf("the response carries %q: %s", kept, body)
		}
	}
}

// A novel nothing is queued for is an empty list, not a missing one.
func TestPendingChangesOfAnUntouchedNovelAreAnEmptyList(t *testing.T) {
	a, _, pending := pendingAPI(t)
	response := readPending(a, "2147483647", "", nil)
	if response.Code != 200 || strings.TrimSpace(response.Body.String()) != `{"items":[],"total":0}` {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if pending.novel != 2147483647 || pending.offset != 0 {
		t.Fatalf("novel=%d offset=%d", pending.novel, pending.offset)
	}
}

func TestPendingChangesNeedASessionThatMayEdit(t *testing.T) {
	a, _, pending := pendingAPI(t)
	if response := readPending(a, "17", "", func(r *http.Request) { r.Header.Del("Cookie") }); response.Code != 401 || errorCode(t, response) != "login_required" || pending.calls != 0 {
		t.Fatalf("without a session: status=%d body=%s calls=%d", response.Code, response.Body, pending.calls)
	}
	for _, scenario := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"an account that may not edit", store.ErrATHubPermissionDenied, 403, "operation_forbidden"},
		{"a forbidden account", store.ErrUserForbidden, 403, "operation_forbidden"},
		{"an outage", errors.New("database is down"), 503, "permissions_unavailable"},
	} {
		a, _, pending := pendingAPI(t)
		// Whatever the store returns next to its refusal is not served.
		pending.err = scenario.err
		pending.list = store.ATHubPendingChangeList{Total: 1, Items: []store.ATHubPendingChange{{ID: uuid.New(), Baseline: json.RawMessage(`{"v":1,"novels":{"17":[{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"旧"},"after":{"title":"不该出现的标题"}}]}}`)}}}
		response := readPending(a, "17", "", nil)
		if response.Code != scenario.status || errorCode(t, response) != scenario.code || strings.Contains(response.Body.String(), "不该出现的标题") {
			t.Fatalf("%s: status=%d body=%s", scenario.name, response.Code, response.Body)
		}
	}
}

func TestPendingChangesRejectMalformedRequests(t *testing.T) {
	for _, scenario := range []struct{ novel, query, code string }{
		{"0", "", "invalid_novel_id"},
		{"-3", "", "invalid_novel_id"},
		{"abc", "", "invalid_novel_id"},
		{"17x", "", "invalid_novel_id"},
		{"1.5", "", "invalid_novel_id"},
		{"2147483648", "", "invalid_novel_id"},
		{"17", "offset=-1", "invalid_offset"},
		{"17", "offset=", "invalid_offset"},
		{"17", "offset=ten", "invalid_offset"},
		{"17", "offset=1000001", "invalid_offset"},
		{"17", "offset=0&offset=50", "invalid_query"},
		{"17", "status=approved", "invalid_query"},
		{"17", "offset=0&user=someone", "invalid_query"},
	} {
		a, _, pending := pendingAPI(t)
		response := readPending(a, scenario.novel, scenario.query, nil)
		if response.Code != 400 || errorCode(t, response) != scenario.code || pending.calls != 0 {
			t.Fatalf("novel=%q query=%q: status=%d body=%s calls=%d", scenario.novel, scenario.query, response.Code, response.Body, pending.calls)
		}
	}
	a, _, pending := pendingAPI(t)
	if response := readPending(a, "17", "offset=1000000", nil); response.Code != 200 || pending.offset != 1000000 {
		t.Fatalf("the largest offset: status=%d body=%s offset=%d", response.Code, response.Body, pending.offset)
	}
}

// Only the creator database knows which queued changes touch a novel, and only
// the content library can read what they do. Without either, nothing is served.
func TestPendingChangesNeedTheCreatorDatabaseAndTheContentLibrary(t *testing.T) {
	for name, prepare := range map[string]func(*API, *keyStore){
		"no creator database": func(a *API, _ *keyStore) { a.reviews = nil },
		// Such a queue cannot tell which of its changes touch a novel.
		"a review store without baselines": func(a *API, keys *keyStore) { a.reviews = &policyAPIStore{keyStore: keys} },
		"no content library":               func(a *API, _ *keyStore) { a.SetATHubChangeReader(nil) },
	} {
		a, keys, pending := pendingAPI(t)
		prepare(a, keys)
		response := readPending(a, "17", "", nil)
		if response.Code != 503 || errorCode(t, response) != "reviews_unavailable" || pending.calls != 0 {
			t.Fatalf("%s: status=%d body=%s calls=%d", name, response.Code, response.Body, pending.calls)
		}
	}
}

// A baseline that cannot be read is an error. Serving the rest would show an
// editor fewer changes than are queued, as if that were all of them.
func TestPendingChangesFailOnABaselineThatCannotBeRead(t *testing.T) {
	for name, baseline := range map[string]string{
		"another version": `{"v":2,"novels":{"17":[{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"旧"},"after":{"title":"读不了的标题"}}]}}`,
		"not a baseline":  `["17"]`,
		"no JSON":         `{"v":1,`,
	} {
		a, _, pending := pendingAPI(t)
		readable := json.RawMessage(`{"v":1,"novels":{"17":[{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"旧"},"after":{"title":"读得了的标题"}}]}}`)
		pending.list = store.ATHubPendingChangeList{Total: 2, Items: []store.ATHubPendingChange{
			{ID: uuid.New(), Baseline: readable}, {ID: uuid.New(), Baseline: json.RawMessage(baseline)},
		}}
		response := readPending(a, "17", "", nil)
		if response.Code != 500 || errorCode(t, response) != "server_error" || strings.Contains(response.Body.String(), "标题") {
			t.Fatalf("%s: status=%d body=%s", name, response.Code, response.Body)
		}
	}
}
