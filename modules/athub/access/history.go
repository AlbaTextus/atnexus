// Compatibility adapter for the existing content access pipeline.
package athubaccess

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/AlbaTextus/atnexus/modules/creatplat"
	"github.com/google/uuid"
)

func (a *Access) RecordChanges(ctx context.Context, drafts []athub.RevisionDraft) (func(context.Context, bool) error, error) {
	return creatplat.NewHistory(a.historyStore(), a.logger).RecordChanges(ctx, drafts)
}

func (a *Access) historyStore() any {
	if a.creatorHistory != nil {
		return a.creatorHistory
	}
	// Isolated legacy fixtures use their own explicit in-memory history store.
	return a.store
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

func (a *Access) history(w http.ResponseWriter, r *http.Request) {
	creatplat.NewHistory(a.historyStore(), a.logger).ServeHTTP(w, r)
}

// ServeHistory reuses the existing authorization/accounting pipeline, preserving
// its admission checks and persisted operation labels while exposing v4.
func (a *Access) ServeHistory(w http.ResponseWriter, r *http.Request) {
	if a.wrapped == nil {
		reply(w, 503, "history_unavailable")
		return
	}
	request := r.Clone(r.Context())
	switch r.URL.Path {
	case "/api/v4/history":
		request.URL.Path = "/api/v3/history"
	case "/api/v4/contributors":
		request.URL.Path = "/api/v3/contributors"
	default:
		reply(w, 404, "not_found")
		return
	}
	request.URL.RawPath = ""
	request.RequestURI = request.URL.RequestURI()
	a.wrapped.ServeHTTP(w, request)
}
