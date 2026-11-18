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

func (a *History) RecordChanges(ctx context.Context, batchID string, drafts []athub.RevisionDraft) (func(context.Context, bool) error, error) {
	st, ok := a.store.(store.NovelHistoryStore)
	if !ok {
		return nil, errors.New("novel history store unavailable")
	}
	batch, err := uuid.Parse(batchID)
	if err != nil {
		return nil, errors.New("revision batch must be a UUID")
	}
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

// RevisionReceipts is the content database's record of which batches committed.
type RevisionReceipts interface {
	CommittedRevisions(context.Context, []string) ([]string, error)
	PruneRevisionReceipts(context.Context, time.Time) error
}

const (
	// Far longer than any content request may run, so a batch this old is no
	// longer in flight and its receipt, if any, is already visible.
	revisionSettleAfter = 10 * time.Minute
	revisionSettleEvery = 5 * time.Minute
	revisionSettleBatch = 200
	// Receipts outlive the settling window by a wide margin and are pruned only
	// after a pass that left no older revision pending.
	revisionReceiptLife = 7 * 24 * time.Hour
)

// RunReconciler settles pending revisions until ctx ends.
func (a *History) RunReconciler(ctx context.Context, content RevisionReceipts) {
	ticker := time.NewTicker(revisionSettleEvery)
	defer ticker.Stop()
	for {
		if settled, err := a.Reconcile(ctx, content, time.Now()); err != nil {
			if ctx.Err() == nil {
				a.logger.Error("novel revision reconciliation failed", "error", err)
			}
		} else if settled > 0 {
			a.logger.Warn("settled novel revisions left pending", "batches", settled)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Reconcile completes or fails every revision batch that has been pending for
// longer than the settling window, according to whether its content committed.
func (a *History) Reconcile(ctx context.Context, content RevisionReceipts, now time.Time) (int, error) {
	st, ok := a.store.(interface {
		StaleRevisionBatches(context.Context, time.Time, int) ([]uuid.UUID, error)
		FinishNovelRevisions(context.Context, uuid.UUID, bool) error
	})
	if !ok {
		return 0, errors.New("novel history store cannot be reconciled")
	}
	settled := 0
	for {
		batches, err := st.StaleRevisionBatches(ctx, now.Add(-revisionSettleAfter), revisionSettleBatch)
		if err != nil {
			return settled, err
		}
		if len(batches) == 0 {
			break
		}
		ids := make([]string, len(batches))
		for i, batch := range batches {
			ids[i] = batch.String()
		}
		found, err := content.CommittedRevisions(ctx, ids)
		if err != nil {
			return settled, err
		}
		committed := make(map[uuid.UUID]bool, len(found))
		for _, id := range found {
			if parsed, err := uuid.Parse(id); err == nil {
				committed[parsed] = true
			}
		}
		for _, batch := range batches {
			// Another settler or the late original completion may have won.
			if err := st.FinishNovelRevisions(ctx, batch, committed[batch]); err != nil && !errors.Is(err, store.ErrNotFound) {
				return settled, err
			}
			settled++
		}
		if len(batches) < revisionSettleBatch {
			break
		}
	}
	return settled, content.PruneRevisionReceipts(ctx, now.Add(-revisionReceiptLife))
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
