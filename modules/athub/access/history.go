// Compatibility adapter for the existing content access pipeline.
package athubaccess

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/AlbaTextus/atnexus/modules/creatplat"
	"github.com/google/uuid"
)

func (a *Access) RecordChanges(ctx context.Context, batch string, drafts []athub.RevisionDraft) (func(context.Context, bool) error, error) {
	if err := a.holdNewImages(ctx, drafts); err != nil {
		return nil, err
	}
	finish, err := creatplat.NewHistory(a.historyStore(), a.logger).RecordChanges(ctx, batch, drafts)
	if err != nil {
		return nil, err
	}
	covers := replacedCovers(drafts)
	if len(covers) == 0 {
		return finish, nil
	}
	return func(ctx context.Context, committed bool) error {
		err := finish(ctx, committed)
		if committed {
			// The caller is still waiting, so all of them share one allowance.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			for _, cover := range covers {
				a.releaseImage(ctx, "book/covers", "", cover)
			}
		}
		return err
	}, nil
}

// holdNewImages refuses a change that starts to point at an image the image
// host no longer has, such as a cover that was replaced and deleted while an
// edit form still showed it. The images it accepts cannot be removed before
// the change has ended. It runs inside the change's transaction.
func (a *Access) holdNewImages(ctx context.Context, drafts []athub.RevisionDraft) error {
	if a.images == nil || a.imageUse == nil {
		return nil
	}
	// A file is held under the address its removal uses, however the change
	// spells it. Images of other hosts are not this host's to vouch for.
	hosted := make(map[string]bool)
	for _, draft := range drafts {
		for _, change := range draft.Changes {
			field, ok := imageFields[change.Entity]
			if !ok {
				continue
			}
			before, _ := change.Before[field].(string)
			after, _ := change.After[field].(string)
			if after == "" || after == before {
				continue
			}
			if address := a.images.Address(after); address != "" {
				hosted[address] = true
			}
		}
	}
	// In a fixed order, so that two changes never wait for each other.
	for _, address := range slices.Sorted(maps.Keys(hosted)) {
		if err := a.imageUse.HoldImage(ctx, address); err != nil {
			return err
		}
		missing, err := a.images.Missing(ctx, address)
		if err != nil {
			return err
		}
		if missing {
			return athub.ErrImageMissing
		}
	}
	return nil
}

// imageFields names where a change carries the address of a stored image.
var imageFields = map[string]string{"Novel": "coverUrl", "Illustration": "url"}

// replacedCovers lists the covers that these changes take away from a novel.
func replacedCovers(drafts []athub.RevisionDraft) []string {
	var covers []string
	for _, draft := range drafts {
		for _, change := range draft.Changes {
			if change.Entity != "Novel" || change.Action == "INSERT" {
				continue
			}
			before, _ := change.Before["coverUrl"].(string)
			after, _ := change.After["coverUrl"].(string)
			if before != "" && before != after {
				covers = append(covers, before)
			}
		}
	}
	return covers
}

// ReleaseImage deletes a stored image that content has stopped needing. The
// image is kept while a novel or illustration points to it or a pending review
// carries its address, and whenever that cannot be established. It is best
// effort: what led here is already saved, so a failure only leaves an unused
// file. It outlives the request that led here, within a short allowance.
func (a *Access) ReleaseImage(ctx context.Context, category, entityID, location string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	a.releaseImage(ctx, category, entityID, location)
}

func (a *Access) releaseImage(ctx context.Context, category, entityID, location string) {
	pending, ok := a.creatorReviews.(interface {
		PendingReviewMentions(context.Context, string) (bool, error)
	})
	// Only the address a file was stored under names it for removal: that is
	// the spelling changes hold it by, and the one its references are found by.
	if !ok || a.images == nil || a.imageUse == nil || location == "" || a.images.Address(location) != location {
		return
	}
	err := a.imageUse.ReleaseImage(ctx, location, func(ctx context.Context) error {
		awaited, err := pending.PendingReviewMentions(ctx, location)
		if err != nil || awaited {
			return err
		}
		return a.images.Remove(ctx, category, entityID, location)
	})
	if err != nil {
		a.logger.Warn("unused image was kept", "category", category, "error", err)
	}
}

// ReconcileRevisions settles revisions whose completion was lost, until ctx ends.
func (a *Access) ReconcileRevisions(ctx context.Context, content creatplat.RevisionReceipts) {
	creatplat.NewHistory(a.historyStore(), a.logger).RunReconciler(ctx, content)
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
	history := creatplat.NewHistory(a.historyStore(), a.logger)
	// Revisions keep the avatar address their contributor had at the time, which
	// may since have been replaced and deleted. Show the current one instead.
	if accounts, ok := a.store.(interface {
		UserAvatars(context.Context, []uuid.UUID) (map[uuid.UUID]string, error)
	}); ok {
		history.ShowCurrentAvatars(accounts.UserAvatars)
	}
	history.ServeHTTP(w, r)
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
