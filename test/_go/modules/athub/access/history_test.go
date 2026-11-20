package athubaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/pkg/auth"
	"github.com/google/uuid"
)

// historyReads stands in for the creator database's history and notes which
// novel was asked for.
type historyReads struct {
	store.ATHubReviewStore
	novelID int32
}

func (s *historyReads) PrepareNovelRevisions(context.Context, uuid.UUID, []store.NovelRevision) error {
	return nil
}
func (s *historyReads) FinishNovelRevisions(context.Context, uuid.UUID, bool) error { return nil }
func (s *historyReads) NovelHistory(_ context.Context, id int32, _, _ int) (store.NovelHistory, error) {
	s.novelID = id
	return store.NovelHistory{Revisions: []store.NovelRevision{}}, nil
}
func (s *historyReads) NovelContributors(_ context.Context, id int32) (json.RawMessage, error) {
	s.novelID = id
	return json.RawMessage(`{"creator":null,"contributors":[]}`), nil
}

// accountHistory is an account store that could answer history itself, as the
// account database once did.
type accountHistory struct {
	accessStore
	historyReads
}

func TestV3HistoryAuthenticatedRoutes(t *testing.T) {
	identity := auth.Identity{UserID: uuid.New(), SessionID: uuid.New()}
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/v3/history?novelId=2", 200},
		{"/api/v3/contributors?novelId=2", 200},
		{"/api/v3/history?novelId=0", 400},
		{"/api/v3/history?novelId=2&novelId=3", 400},
		{"/api/v3/history?novelId=2&limit=101", 400},
		{"/api/v3/history?novelId=2&offset=-1", 400},
		{"/api/v3/contributors?novelId=2&limit=20", 400},
		{"/api/v2/history?novelId=2", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			st, creator := &accessStore{}, &historyReads{}
			access, err := New(st, ":8003", nil)
			if err != nil {
				t.Fatal(err)
			}
			access.EnableCreatorStorage(creator)
			access.EnableSessions(sessionVerifier{identity: identity})
			handler := access.Wrap(http.NotFoundHandler())
			r := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || !st.finished {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status == 200 && (creator.novelID != 2 || st.call.Path != strings.Split(tc.path, "?")[0]) {
				t.Fatal("history query/audit route wrong")
			}
		})
	}
	for _, denied := range []bool{false, true} {
		st, creator := &accessStore{}, &historyReads{}
		verifier := sessionVerifier{err: auth.ErrUnauthenticated}
		if denied {
			st.beginErr = store.ErrATHubAccessDenied
			verifier = sessionVerifier{identity: identity}
		}
		access, _ := New(st, ":8003", nil)
		access.EnableCreatorStorage(creator)
		access.EnableSessions(verifier)
		r := httptest.NewRequest("GET", "/api/v3/history?novelId=2", nil)
		w := httptest.NewRecorder()
		access.Wrap(http.NotFoundHandler()).ServeHTTP(w, r)
		if w.Code != 401 || creator.novelID != 0 {
			t.Fatal("unauthenticated/disabled history access accepted")
		}
	}
}

// History is kept by the creator database only. A pipeline without one answers
// that history is unavailable, even over an account store that could answer.
func TestHistoryNeedsTheCreatorDatabase(t *testing.T) {
	for _, path := range []string{"/api/v3/history?novelId=2", "/api/v3/contributors?novelId=2"} {
		st := &accountHistory{}
		access, err := New(st, ":8003", nil)
		if err != nil {
			t.Fatal(err)
		}
		access.EnableSessions(sessionVerifier{identity: auth.Identity{UserID: uuid.New(), SessionID: uuid.New()}})
		w := httptest.NewRecorder()
		access.Wrap(http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 503 || !strings.Contains(w.Body.String(), "history_unavailable") {
			t.Fatalf("%s: status=%d body=%s", path, w.Code, w.Body)
		}
		if st.novelID != 0 {
			t.Fatalf("%s: history was read from the account store", path)
		}
		if !st.finished || st.status != 503 {
			t.Fatalf("%s: refusal was not recorded: finished=%v status=%d", path, st.finished, st.status)
		}
	}
}
