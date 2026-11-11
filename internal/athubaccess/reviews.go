package athubaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"

	"albatextus/athub"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

func (a *Access) queueMutation(w http.ResponseWriter, r *http.Request, call store.ATHubCall, actor athub.Contributor) bool {
	forbid := func() {
		if st, ok := a.store.(interface {
			RejectATHubAdmission(context.Context, uuid.UUID) error
		}); ok {
			if err := st.RejectATHubAdmission(r.Context(), call.ID); err != nil {
				reply(w, 503, "accounting_unavailable")
				return
			}
		}
		reply(w, 403, "operation_forbidden")
	}
	st, ok := a.store.(store.ATHubPolicyStore)
	if !ok {
		return false
	} // Alternate/test stores explicitly opt into policy support.
	users, ok := a.store.(interface {
		UserByID(context.Context, uuid.UUID) (store.User, error)
	})
	if !ok {
		reply(w, 503, "permissions_unavailable")
		return true
	}
	user, err := users.UserByID(r.Context(), *call.UserID)
	if err != nil {
		reply(w, 503, "permissions_unavailable")
		return true
	}
	policy, err := st.ATHubPolicy(r.Context(), user.Role)
	if err != nil {
		reply(w, 503, "permissions_unavailable")
		return true
	}
	op := store.ATHubOperation(r.Method, r.URL.Path)
	if !user.IsActive || !policy.Rule(op, r.Method, r.URL.Path).Allowed {
		forbid()
		return true
	}
	var uploadBody []byte
	if r.URL.Path == "/api/v2/upload" {
		uploadBody, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			reply(w, 413, "payload_too_large")
			return true
		}
		for _, edit := range store.ATHubUploadEdits(r.URL.Path, uploadBody) {
			if !policy.Rule(edit.Operation, edit.Method, edit.Path).Allowed {
				forbid()
				return true
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(uploadBody))
	}
	if !policy.ReviewRequired {
		return false
	}
	reviews, ok := a.store.(store.ATHubReviewStore)
	if !ok {
		reply(w, 503, "reviews_unavailable")
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	allowedQuery := map[string]bool{}
	if r.Method == http.MethodDelete || r.Method == http.MethodPut {
		if r.URL.Path == "/api/v2/content" || r.URL.Path == "/api/v2/volumes" {
			if r.Method == http.MethodDelete {
				allowedQuery = map[string]bool{"novelId": true, "order": true}
			}
		} else {
			allowedQuery["id"] = true
		}
	}
	if err != nil {
		reply(w, 400, "invalid_query")
		return true
	}
	for key, values := range query {
		if !allowedQuery[key] || len(values) != 1 {
			reply(w, 400, "invalid_query")
			return true
		}
	}
	payload := []byte(`{}`)
	if r.Method != http.MethodDelete {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			reply(w, 400, "invalid_json")
			return true
		}
		payload, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			reply(w, 413, "payload_too_large")
			return true
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(payload, &object) != nil || object == nil {
			reply(w, 400, "invalid_json")
			return true
		}
	}
	contributor, _ := json.Marshal(actor)
	item, err := reviews.QueueATHubReview(r.Context(), store.ATHubReview{
		ID: call.ID, UserID: call.UserID, Contributor: contributor, Operation: op, Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Payload: payload,
	})
	if err != nil {
		if errors.Is(err, store.ErrATHubPermissionDenied) {
			forbid()
		} else {
			reply(w, 503, "reviews_unavailable")
		}
		return true
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"review_id": item.ID, "status": "pending", "message": "已提交审核，通过后生效。"})
	return true
}

type reviewResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *reviewResponse) Header() http.Header { return w.header }
func (w *reviewResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *reviewResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.body.Write(body)
}

// ApplyReview invokes the private content handler, with a host-only receipt ID.
// It never accepts a client-supplied bypass header or forwards account cookies.
func (a *Access) ApplyReview(ctx context.Context, item store.ATHubReview) (store.ATHubReviewResult, error) {
	if a.content == nil || item.UserID == nil {
		return store.ATHubReviewResult{}, errors.New("review executor unavailable")
	}
	if store.ATHubOperation(item.Method, item.Path) != item.Operation || (item.Operation != "upload" && item.Operation != "edit" && item.Operation != "delete") {
		return store.ATHubReviewResult{}, store.ErrATHubPermissionDenied
	}
	var actor athub.Contributor
	if json.Unmarshal(item.Contributor, &actor) != nil || actor.ID != item.UserID.String() {
		return store.ATHubReviewResult{}, store.ErrATHubPermissionDenied
	}
	ctx = athub.WithContributor(ctx, actor)
	ctx = athub.WithMutationID(ctx, item.ID.String())
	var result store.ATHubReviewResult
	for attempt := 0; attempt < 2; attempt++ {
		target := item.Path
		if item.Query != "" {
			target += "?" + item.Query
		}
		request, err := http.NewRequestWithContext(ctx, item.Method, target, bytes.NewReader(item.Payload))
		if err != nil {
			return result, err
		}
		request.Header.Set("Content-Type", "application/json")
		response := &reviewResponse{header: make(http.Header)}
		a.content.ServeHTTP(response, request)
		result = store.ATHubReviewResult{Status: response.status, Body: json.RawMessage(response.body.Bytes())}
		if result.Status == 0 {
			result.Status = 200
		}
		if !json.Valid(result.Body) {
			result.Body = json.RawMessage(`null`)
		}
		if result.Status < 500 || ctx.Err() != nil {
			break
		}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, nil
}
