package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type decidedReviewStore struct {
	*policyAPIStore
	decided store.ATHubReview
}

func (s *decidedReviewStore) DecideATHubReview(_ context.Context, _ uuid.UUID, id uuid.UUID, _ string, _ string, _ store.ATHubReviewExecutor) (store.ATHubReview, error) {
	s.decided.ID = id
	return s.decided, nil
}

// A decided illustration upload hands its image to the host, which deletes it
// unless it became an illustration. Other reviews hand over nothing.
func TestDecidedIllustrationReviewReleasesItsImage(t *testing.T) {
	const location = "https://images.test/file/book/17/illustrations/1.jpg"
	payload, _ := json.Marshal(map[string]any{"url": location, "sha256": "x", "width": 1, "height": 1})
	illustration := store.ATHubReview{Method: "POST", Path: "/api/v2/illustrations", Query: "novelId=17", Payload: payload}
	for _, scenario := range []struct {
		name     string
		review   func(*store.ATHubReview)
		released bool
	}{
		{"rejected", func(r *store.ATHubReview) { r.Status = "rejected" }, true},
		{"refused by content", func(r *store.ATHubReview) { r.Status = "failed" }, true},
		// Recorded or a duplicate: the host tells them apart by what refers to it.
		{"approved", func(r *store.ATHubReview) { r.Status = "approved" }, true},
		{"another kind of change", func(r *store.ATHubReview) { r.Status, r.Method, r.Path = "rejected", "PUT", "/api/v2/novelinfo" }, false},
		{"removal of an illustration", func(r *store.ATHubReview) { r.Status, r.Method = "approved", "DELETE" }, false},
		{"no novel", func(r *store.ATHubReview) { r.Status, r.Query = "rejected", "" }, false},
		{"no address", func(r *store.ATHubReview) { r.Status, r.Payload = "rejected", json.RawMessage(`{"sha256":"x"}`) }, false},
		{"unreadable upload", func(r *store.ATHubReview) { r.Status, r.Payload = "rejected", json.RawMessage(`[]`) }, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a, keys := keyAPI(t)
			keys.user.Role = "ADMIN"
			decided := illustration
			scenario.review(&decided)
			s := &decidedReviewStore{policyAPIStore: &policyAPIStore{keyStore: keys}, decided: decided}
			a.Store, a.reviews = s, s
			var released []string
			a.SetImageRelease(func(_ context.Context, category, entityID, location string) {
				released = append(released, category+" "+entityID+" "+location)
			})
			reviewID := uuid.NewString()
			request := keyRequest("POST", `{"decision":"reject","note":"重复"}`)
			request.URL.Path = "/api/v4/admin/athub/reviews/" + reviewID
			params := chi.NewRouteContext()
			params.URLParams.Add("id", reviewID)
			w := httptest.NewRecorder()
			a.athubReviews(w, request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, params)))
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			want := []string(nil)
			if scenario.released {
				want = []string{"book/illustrations 17 " + location}
			}
			if !slices.Equal(released, want) {
				t.Fatalf("released=%v", released)
			}
		})
	}
}

// A novel keeps each picture once. Uploading one it already has answers with
// the existing illustration, so the file just stored is unused.
func TestDuplicateIllustrationUploadDropsTheNewFile(t *testing.T) {
	stored := &illustrationUpload{ResponseWriter: httptest.NewRecorder(), novel: "17", location: "https://images.test/book/illustrations/17/2.jpg"}
	for reply, removed := range map[string]bool{
		`{"id":5,"url":"https://images.test/book/illustrations/17/1.jpg"}`: true,
		`{"id":6,"url":"https://images.test/book/illustrations/17/2.jpg"}`: false,
		`{"id":7}`:     false,
		`not json`:     false,
		`{"url":""}`:   false,
		`{"url":null}`: false,
	} {
		a, _ := keyAPI(t)
		host := &imageHost{}
		a.Images = host
		upload := *stored
		upload.WriteHeader(http.StatusCreated)
		_, _ = upload.Write([]byte(reply))
		a.discardRefusedIllustration(context.Background(), &upload)
		if (len(host.removed) == 1) != removed {
			t.Errorf("%s: removed=%v", reply, host.removed)
		}
	}
}
