package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlbaTextus/atnexus/internal/security"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (a *API) SetATHubReviewExecutor(execute store.ATHubReviewExecutor) {
	a.athubReviewExecutor = execute
}

func (a *API) athubReviews(w http.ResponseWriter, r *http.Request) {
	user, ok := a.AccountActor(w, r)
	if !ok {
		return
	}
	st, ok := a.Store.(store.ATHubReviewStore)
	if a.reviews != nil {
		st, ok = a.reviews, true
	}
	if !ok {
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
		audit, ok := a.beginReviewAudit(w, r, user.ID)
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
	// Repeated contributors/reviewers share one lookup within a list response.
	avatars := make(map[uuid.UUID]*string)
	populate := func(identity *json.RawMessage, accountID *uuid.UUID) {
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
		avatar, cached := avatars[id]
		if !cached {
			user, err := a.Store.UserByID(ctx, id)
			if err == nil {
				avatar = &user.AvatarURL
			} else if !errors.Is(err, store.ErrNotFound) {
				a.Log.Warn("review identity avatar lookup failed", "user", id, "error", err)
			}
			avatars[id] = avatar
		}
		if avatar != nil {
			person["avatar"], _ = json.Marshal(*avatar)
			*identity, _ = json.Marshal(person)
		}
	}
	for _, item := range items {
		populate(&item.Contributor, item.UserID)
		populate(&item.Reviewer, nil)
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
func (a *API) beginReviewAudit(w http.ResponseWriter, r *http.Request, user uuid.UUID) (*reviewAuditWriter, bool) {
	st, ok := a.Store.(store.ATHubStore)
	if !ok {
		a.PolicyError(w, errors.New("audit store unavailable"))
		return nil, false
	}
	cookie, err := r.Cookie(a.Config.SessionCookieName)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return nil, false
	}
	session, err := a.Store.SessionByHash(r.Context(), security.HashToken(cookie.Value))
	if err != nil || session.UserID != user {
		writeJSON(w, 401, map[string]string{"error": "login_required"})
		return nil, false
	}
	_, portText, err := net.SplitHostPort(a.Config.CreatplatHTTPAddr)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "invalid_creator_address"})
		return nil, false
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		writeJSON(w, 503, map[string]string{"error": "invalid_creator_address"})
		return nil, false
	}
	// The existing admission adapter still uses its persisted review operation label.
	call := store.ATHubCall{ID: uuid.New(), RequestID: uuid.New(), UserID: &user, SessionID: &session.ID, Method: "POST", Path: "/api/v3/reviews", Port: port, Authenticated: true}
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
