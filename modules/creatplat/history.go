package creatplat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/google/uuid"
)

// History owns contribution recording and revision queries. The current account
// database remains the transitional persistence adapter.
type History struct {
	store  any
	logger *slog.Logger
}

func NewHistory(st any, logger *slog.Logger) *History {
	if logger == nil {
		logger = slog.Default()
	}
	return &History{store: st, logger: logger}
}

func (a *History) RecordChanges(ctx context.Context, drafts []athub.RevisionDraft) (func(context.Context, bool) error, error) {
	st, ok := a.store.(store.NovelHistoryStore)
	if !ok {
		return nil, errors.New("novel history store unavailable")
	}
	batch := uuid.New()
	revisions := make([]store.NovelRevision, 0, len(drafts))
	for _, draft := range drafts {
		userID, err := uuid.Parse(draft.Contributor.ID)
		if err != nil {
			return nil, errors.New("authenticated contributor required")
		}
		actor, err := json.Marshal(draft.Contributor)
		if err != nil {
			return nil, err
		}
		changes, err := json.Marshal(draft.Changes)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, store.NovelRevision{
			ID: uuid.New(), NovelID: draft.NovelID, UserID: &userID, Contributor: actor, Changes: changes,
		})
	}
	if err := st.PrepareNovelRevisions(ctx, batch, revisions); err != nil {
		return nil, err
	}
	return func(ctx context.Context, committed bool) error {
		err := st.FinishNovelRevisions(ctx, batch, committed)
		if err != nil {
			a.logger.Error("novel revision completion pending", "batch_id", batch, "content_committed", committed)
		}
		return err
	}, nil
}

func httpQuery(r *http.Request) (url.Values, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	for key, values := range query {
		if len(values) != 1 || (key != "novelId" && (r.URL.Path == "/api/v3/contributors" || (key != "limit" && key != "offset"))) {
			return nil, errors.New("unknown or repeated parameter")
		}
	}
	return query, nil
}

// History uses the service DB and the same authentication gate as content.
func (a *History) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		apibase.WriteError(w, 405, "method_not_allowed")
		return
	}
	query, err := httpQuery(r)
	if err != nil {
		apibase.WriteError(w, 400, "invalid_query")
		return
	}
	id, err := strconv.ParseInt(query.Get("novelId"), 10, 32)
	if err != nil || id <= 0 {
		apibase.WriteError(w, 400, "invalid_novel_id")
		return
	}
	limit, offset := 20, 0
	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			apibase.WriteError(w, 400, "invalid_limit")
			return
		}
	}
	if query.Has("offset") {
		offset, err = strconv.Atoi(query.Get("offset"))
		if err != nil || offset < 0 || offset > 2147483647 {
			apibase.WriteError(w, 400, "invalid_offset")
			return
		}
	}
	st, ok := a.store.(store.NovelHistoryStore)
	if !ok {
		apibase.WriteError(w, 503, "history_unavailable")
		return
	}
	var result any
	if r.URL.Path == "/api/v3/contributors" {
		result, err = st.NovelContributors(r.Context(), int32(id))
	} else {
		result, err = st.NovelHistory(r.Context(), int32(id), limit, offset)
	}
	if err != nil {
		a.logger.Error("novel history query failed")
		apibase.WriteError(w, 503, "history_unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}
