package athubaccess

import (
	"context"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/AlbaTextus/atnexus/modules/creatplat"
)

func (a *Access) queueMutation(w http.ResponseWriter, r *http.Request, call store.ATHubCall, actor athub.Contributor) bool {
	return creatplat.NewGovernance(a.store, a.content, a.creatorReviews).QueueMutation(w, r, call, actor)
}

func (a *Access) ApplyReview(ctx context.Context, item store.ATHubReview) (store.ATHubReviewResult, error) {
	return creatplat.NewGovernance(a.store, a.content).ApplyReview(ctx, item)
}
