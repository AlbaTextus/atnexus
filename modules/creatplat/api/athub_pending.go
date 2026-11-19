package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// SetATHubChangeReader supplies the content library's reading of a baseline:
// what the change it was queued with does to one novel. Without it the changes
// awaiting a decision cannot be described, and are not served.
func (a *API) SetATHubChangeReader(read func(baseline []byte, novelID int32) (any, error)) {
	a.athubChangeReader = read
}

// pendingChange is one submission awaiting a decision, as it is shown to
// whoever edits the novel it alters: when it was submitted and what it does to
// that novel. It names no submitter and carries no baseline.
type pendingChange struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Changes   any       `json:"changes"`
}

// pendingNovelChanges serves the submissions awaiting a decision that alter a
// novel, oldest first, to an account that may edit novels.
func (a *API) pendingNovelChanges(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.SessionActor(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_novel_id"})
		return
	}
	query := r.URL.Query()
	for key, values := range query {
		if key != "offset" || len(values) != 1 {
			writeJSON(w, 400, map[string]string{"error": "invalid_query"})
			return
		}
	}
	offset := 0
	if query.Has("offset") {
		value, err := strconv.Atoi(query.Get("offset"))
		if err != nil || value < 0 || value > 1000000 {
			writeJSON(w, 400, map[string]string{"error": "invalid_offset"})
			return
		}
		offset = value
	}
	st, ok := a.reviews.(store.ATHubPendingChangeStore)
	if !ok || a.athubChangeReader == nil {
		writeJSON(w, 503, map[string]string{"error": "reviews_unavailable"})
		return
	}
	result, err := st.PendingATHubChanges(r.Context(), user.ID, int32(id), offset)
	if err != nil {
		a.PolicyError(w, err)
		return
	}
	items := make([]pendingChange, 0, len(result.Items))
	for _, item := range result.Items {
		changes, err := a.athubChangeReader(item.Baseline, int32(id))
		if err != nil {
			a.Log.Error("a pending change could not be read", "review", item.ID, "error", err)
			writeJSON(w, 500, map[string]string{"error": "server_error"})
			return
		}
		items = append(items, pendingChange{ID: item.ID, CreatedAt: item.CreatedAt, Changes: changes})
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": result.Total})
}
