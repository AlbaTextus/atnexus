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

type historyAccessStore struct {
	accessStore
	novelID int32
}

func (s *historyAccessStore) PrepareNovelRevisions(context.Context, uuid.UUID, []store.NovelRevision) error {
	return nil
}
func (s *historyAccessStore) FinishNovelRevisions(context.Context, uuid.UUID, bool) error { return nil }
func (s *historyAccessStore) NovelHistory(_ context.Context, id int32, _, _ int) (store.NovelHistory, error) {
	s.novelID = id
	return store.NovelHistory{Revisions: []store.NovelRevision{}}, nil
}
func (s *historyAccessStore) NovelContributors(_ context.Context, id int32) (json.RawMessage, error) {
	s.novelID = id
	return json.RawMessage(`{"creator":null,"contributors":[]}`), nil
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
			st := &historyAccessStore{}
			access, err := New(st, ":8003", nil)
			if err != nil {
				t.Fatal(err)
			}
			access.EnableSessions(sessionVerifier{identity: identity})
			handler := access.Wrap(http.NotFoundHandler())
			r := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || !st.finished {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status == 200 && (st.novelID != 2 || st.call.Path != strings.Split(tc.path, "?")[0]) {
				t.Fatal("history query/audit route wrong")
			}
		})
	}
	for _, denied := range []bool{false, true} {
		st := &historyAccessStore{}
		verifier := sessionVerifier{err: auth.ErrUnauthenticated}
		if denied {
			st.beginErr = store.ErrATHubAccessDenied
			verifier = sessionVerifier{identity: identity}
		}
		access, _ := New(st, ":8003", nil)
		access.EnableSessions(verifier)
		r := httptest.NewRequest("GET", "/api/v3/history?novelId=2", nil)
		w := httptest.NewRecorder()
		access.Wrap(http.NotFoundHandler()).ServeHTTP(w, r)
		if w.Code != 401 || st.novelID != 0 {
			t.Fatal("unauthenticated/disabled history access accepted")
		}
	}
}
