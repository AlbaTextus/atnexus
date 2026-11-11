package athubaccess

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"albatextus/athub"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
)

func (a *Access) RecordChanges(ctx context.Context, drafts []athub.RevisionDraft) (func(context.Context, bool) error, error) {
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

func (a *Access) contributor(ctx context.Context, userID uuid.UUID) (athub.Contributor, error) {
	st, ok := a.store.(interface {
		UserByID(context.Context, uuid.UUID) (store.User, error)
	})
	if !ok {
		return athub.Contributor{}, errors.New("contributor lookup unavailable")
	}
	user, err := st.UserByID(ctx, userID)
	if err != nil {
		return athub.Contributor{}, err
	}
	name := "用户 #" + strconv.Itoa(user.NumericID)
	if user.DisplayName != nil && *user.DisplayName != "" {
		name = *user.DisplayName
	}
	return athub.Contributor{ID: user.ID.String(), Name: name, Avatar: user.AvatarURL}, nil
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
func (a *Access) history(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		reply(w, 405, "method_not_allowed")
		return
	}
	query, err := httpQuery(r)
	if err != nil {
		reply(w, 400, "invalid_query")
		return
	}
	id, err := strconv.ParseInt(query.Get("novelId"), 10, 32)
	if err != nil || id <= 0 {
		reply(w, 400, "invalid_novel_id")
		return
	}
	limit, offset := 20, 0
	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			reply(w, 400, "invalid_limit")
			return
		}
	}
	if query.Has("offset") {
		offset, err = strconv.Atoi(query.Get("offset"))
		if err != nil || offset < 0 || offset > 2147483647 {
			reply(w, 400, "invalid_offset")
			return
		}
	}
	st, ok := a.store.(store.NovelHistoryStore)
	if !ok {
		reply(w, 503, "history_unavailable")
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
		reply(w, 503, "history_unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}
