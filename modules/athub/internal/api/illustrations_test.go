package api

import (
	"context"
	"net/http"
	"testing"

	"albatextus/athub/internal/storage"
)

type deleteIllustrationStore struct {
	fakeStore
	novelID, imageID int32
}

func (s *deleteIllustrationStore) DeleteIllustration(_ context.Context, novelID, imageID int32) error {
	s.called, s.novelID, s.imageID = "deleteIllustration", novelID, imageID
	return s.err
}

func TestDeleteIllustrationValidationAndErrors(t *testing.T) {
	for _, scenario := range []struct {
		name, query string
		err         error
		want        int
		called      bool
	}{
		{"valid", "novelId=17&id=23", nil, 204, true},
		{"missing image", "novelId=17", nil, 400, false},
		{"missing novel", "id=23", nil, 400, false},
		{"negative image", "novelId=17&id=-1", nil, 400, false},
		{"zero novel", "novelId=0&id=23", nil, 400, false},
		{"overflow", "novelId=17&id=2147483648", nil, 400, false},
		{"duplicate", "novelId=17&id=23&id=24", nil, 400, false},
		{"unexpected pagination", "novelId=17&id=23&limit=1", nil, 400, false},
		{"wrong parent or missing image", "novelId=17&id=23", storage.ErrNotFound, 404, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			st := &deleteIllustrationStore{fakeStore: fakeStore{err: scenario.err}}
			response := request(testHandler(t, st), http.MethodDelete, "/api/v2/illustrations?"+scenario.query, "", "Bearer "+testToken)
			if response.Code != scenario.want || (st.called == "deleteIllustration") != scenario.called {
				t.Fatalf("status=%d body=%s called=%s", response.Code, response.Body, st.called)
			}
			if scenario.called && (st.novelID != 17 || st.imageID != 23) {
				t.Fatalf("lost parent/image identity: %d/%d", st.novelID, st.imageID)
			}
		})
	}
}
