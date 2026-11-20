package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) SetATHubReviewExecutor(execute store.ATHubReviewExecutor) {
	a.athubReviewExecutor = execute
}

// SetATHubReceiptForgetter releases the content replay receipt of a decided review.
func (a *API) SetATHubReceiptForgetter(forget func(context.Context, string) error) {
	a.athubReceiptForgetter = forget
}

// SetImageRelease supplies the host's way of deleting a stored image once
// nothing refers to it. Without it, decided reviews leave their images.
func (a *API) SetImageRelease(release func(ctx context.Context, category, entityID, location string)) {
	a.releaseImage = release
}

// discardUnrecordedIllustration hands the image of a reviewed illustration
// upload back to the host. An upload that became an illustration is referenced
// and stays. One that was rejected, refused by the content API, or answered
// with the same picture the novel already had under another address is
// referenced by nothing, and is deleted.
func (a *API) discardUnrecordedIllustration(ctx context.Context, item store.ATHubReview) {
	if a.releaseImage == nil || item.Method != http.MethodPost || item.Path != "/api/v2/illustrations" || item.Status == "pending" {
		return
	}
	var upload struct {
		URL string `json:"url"`
	}
	query, err := url.ParseQuery(item.Query)
	if err != nil || query.Get("novelId") == "" || json.Unmarshal(item.Payload, &upload) != nil || upload.URL == "" {
		return
	}
	a.releaseImage(ctx, "book/illustrations", query.Get("novelId"), upload.URL)
}

func (a *API) athubReviews(w http.ResponseWriter, r *http.Request) {
	user, session, ok := a.SessionActor(w, r)
	if !ok {
		return
	}
	st := a.reviews
	if st == nil {
		a.PolicyError(w, errors.New("review store unavailable"))
		return
	}
	all := strings.HasPrefix(r.URL.Path, "/api/v4/admin/")
	if all && !isAdmin(user) {
		a.PolicyError(w, store.ErrUserForbidden)
		return
	}
	if idText := chi.URLParam(r, "id"); idText != "" {
		id, err := uuid.Parse(idText)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_review_id"})
			return
		}
		if r.Method == http.MethodGet {
			item, err := st.ATHubReview(r.Context(), user.ID, id, all)
			if err != nil {
				a.PolicyError(w, err)
				return
			}
			a.populateReviewAvatars(r.Context(), &item)
			writeJSON(w, 200, item)
			return
		}
		var input struct {
			Decision string `json:"decision"`
			Note     string `json:"note"`
		}
		if !readAccountJSON(w, r, &input) {
			return
		}
		input.Note = strings.TrimSpace(input.Note)
		if (input.Decision != "approve" && input.Decision != "reject") || utf8.RuneCountInString(input.Note) > 1000 || (input.Decision == "reject" && input.Note == "") {
			writeJSON(w, 400, map[string]string{"error": "invalid_decision", "error_description": "请选择通过或驳回；驳回时请填写原因，最多 1000 字。"})
			return
		}
		audit, ok := a.beginReviewAudit(w, r, user.ID, session.ID)
		if !ok {
			return
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			if err := a.Store.(store.ATHubStore).FinishATHubCall(ctx, audit.id, audit.status, time.Since(audit.start).Milliseconds(), audit.bytes); err != nil {
				a.Log.Error("review audit completion failed", "id", audit.id)
			}
		}()
		item, err := st.DecideATHubReview(r.Context(), user.ID, id, input.Decision, input.Note, a.athubReviewExecutor)
		if err != nil {
			if errors.Is(err, store.ErrUserForbidden) || errors.Is(err, store.ErrATHubPermissionDenied) {
				if admissions, ok := a.Store.(interface {
					RejectATHubAdmission(context.Context, uuid.UUID) error
				}); ok {
					_ = admissions.RejectATHubAdmission(r.Context(), audit.id)
				}
			}
			a.PolicyError(audit, err)
			return
		}
		// The decision is committed, so this review can never be applied again and
		// its replay receipt is no longer needed. A failed cleanup only leaves the
		// receipt behind; it never affects the decision. A rejected review was
		// never applied, so it has no receipt to release.
		if a.athubReceiptForgetter != nil && input.Decision == "approve" {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			if err := a.athubReceiptForgetter(ctx, id.String()); err != nil {
				a.Log.Warn("review receipt cleanup failed", "review", id, "error", err)
			}
			cancel()
		}
		a.discardUnrecordedIllustration(r.Context(), item)
		a.populateReviewAvatars(r.Context(), &item)
		writeJSON(audit, 200, item)
		return
	}
	query := r.URL.Query()
	status := query.Get("status")
	offset := 0
	for key, values := range query {
		if len(values) != 1 || (key != "status" && key != "offset") {
			writeJSON(w, 400, map[string]string{"error": "invalid_review_query"})
			return
		}
	}
	if query.Has("offset") {
		value, err := strconv.Atoi(query.Get("offset"))
		if err != nil || value < 0 || value > 1000000 {
			writeJSON(w, 400, map[string]string{"error": "invalid_offset"})
			return
		}
		offset = value
	}
	if status != "" && status != "pending" && status != "approved" && status != "rejected" && status != "failed" {
		writeJSON(w, 400, map[string]string{"error": "invalid_review_status"})
		return
	}
	result, err := st.ListATHubReviews(r.Context(), user.ID, all, status, offset)
	if err != nil {
		a.PolicyError(w, err)
		return
	}
	items := make([]*store.ATHubReview, len(result.Items))
	for i := range result.Items {
		items[i] = &result.Items[i]
	}
	a.populateReviewAvatars(r.Context(), items...)
	writeJSON(w, 200, result)
}

// Resolve avatars on read so historical reviews also follow the account's selected image.
func (a *API) populateReviewAvatars(ctx context.Context, items ...*store.ATHubReview) {
	type slot struct {
		identity *json.RawMessage
		person   map[string]json.RawMessage
		id       uuid.UUID
	}
	slots := make([]slot, 0, 2*len(items))
	ids, seen := make([]uuid.UUID, 0), make(map[uuid.UUID]bool)
	collect := func(identity *json.RawMessage, accountID *uuid.UUID) {
		var person map[string]json.RawMessage
		if json.Unmarshal(*identity, &person) != nil || person == nil {
			return
		}
		var id uuid.UUID
		if accountID != nil {
			id = *accountID
		} else if json.Unmarshal(person["id"], &id) != nil {
			return
		}
		if id == uuid.Nil {
			return
		}
		slots = append(slots, slot{identity, person, id})
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, item := range items {
		collect(&item.Contributor, item.UserID)
		collect(&item.Reviewer, nil)
	}
	if len(ids) == 0 {
		return
	}
	// Repeated contributors/reviewers share one lookup, and a store that can
	// batch resolves the whole response in a single query.
	avatars := make(map[uuid.UUID]string, len(ids))
	if batch, ok := a.Store.(interface {
		UserAvatars(context.Context, []uuid.UUID) (map[uuid.UUID]string, error)
	}); ok {
		found, err := batch.UserAvatars(ctx, ids)
		if err != nil {
			a.Log.Warn("review identity avatar lookup failed", "error", err)
			return
		}
		avatars = found
	} else {
		for _, id := range ids {
			user, err := a.Store.UserByID(ctx, id)
			if err == nil {
				avatars[id] = user.AvatarURL
			} else if !errors.Is(err, store.ErrNotFound) {
				a.Log.Warn("review identity avatar lookup failed", "user", id, "error", err)
			}
		}
	}
	for _, s := range slots {
		if avatar, ok := avatars[s.id]; ok {
			s.person["avatar"], _ = json.Marshal(avatar)
			*s.identity, _ = json.Marshal(s.person)
		}
	}
}

type reviewAuditWriter struct {
	http.ResponseWriter
	id     uuid.UUID
	start  time.Time
	status int
	bytes  int64
}

func (w *reviewAuditWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *reviewAuditWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	w.bytes += int64(n)
	return n, err
}
func (a *API) beginReviewAudit(w http.ResponseWriter, r *http.Request, user, session uuid.UUID) (*reviewAuditWriter, bool) {
	st, ok := a.Store.(store.ATHubStore)
	if !ok {
		a.PolicyError(w, errors.New("audit store unavailable"))
		return nil, false
	}
	_, portText, err := net.SplitHostPort(a.Config.GatewayHTTPAddr)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "invalid_gateway_address"})
		return nil, false
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		writeJSON(w, 503, map[string]string{"error": "invalid_gateway_address"})
		return nil, false
	}
	// The existing admission adapter still uses its persisted review operation label.
	call := store.ATHubCall{ID: uuid.New(), RequestID: uuid.New(), UserID: &user, SessionID: &session, Method: "POST", Path: "/api/v3/reviews", Port: port, Authenticated: true}
	err = st.BeginATHubCall(r.Context(), call)
	if err != nil {
		status, code := 503, "accounting_unavailable"
		if errors.Is(err, store.ErrATHubPermissionDenied) || errors.Is(err, store.ErrATHubAccessDenied) {
			status, code = 403, "operation_forbidden"
		}
		if status != 503 {
			_ = st.FinishATHubCall(context.WithoutCancel(r.Context()), call.ID, status, 0, 0)
		}
		writeJSON(w, status, map[string]string{"error": code})
		return nil, false
	}
	return &reviewAuditWriter{ResponseWriter: w, id: call.ID, start: time.Now(), status: 500}, true
}
