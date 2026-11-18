// Package httpapi owns creator submissions, reviews and revisions.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type API struct {
	*apibase.Base
	history                       http.Handler
	reviews                       store.ATHubReviewStore
	creatorPing                   func(context.Context) error
	athubReviewExecutor           store.ATHubReviewExecutor
	athubReceiptForgetter         func(context.Context, string) error
	athubSubmitter                func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID)
	athubNovelEditor              func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)
	athubIllustrationReader       func(http.ResponseWriter, *http.Request, int32)
	athubIllustrationLayoutReader func(http.ResponseWriter, *http.Request, int32)
	athubIllustrationSubmitter    func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)
	athubIllustrationDeleter      func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32, int32)
	athubIllustrationLayoutEditor func(http.ResponseWriter, *http.Request, uuid.UUID, uuid.UUID, int32)
	// Personal-key uploads: the ATHub access pipeline admits the call, then runs
	// the given step to store the file.
	athubKeyCoverUploader        func(http.ResponseWriter, *http.Request, func(http.ResponseWriter, *http.Request, uuid.UUID))
	athubKeyIllustrationUploader func(http.ResponseWriter, *http.Request, func(http.ResponseWriter, *http.Request) (int32, []byte, bool))
}

func New(cfg config.Config, st store.Store, logger *slog.Logger) *API {
	if logger == nil {
		logger = slog.Default()
	}
	return &API{Base: &apibase.Base{Config: cfg, Store: st, Log: logger}}
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v4/healthz", func(w http.ResponseWriter, r *http.Request) {
		if a.creatorPing == nil {
			writeJSON(w, 503, map[string]string{"status": "creator_database_unavailable"})
			return
		}
		if err := a.creatorPing(r.Context()); err != nil {
			writeJSON(w, 503, map[string]string{"status": "unhealthy"})
			return
		}
		if err := a.Store.Ping(r.Context()); err != nil {
			writeJSON(w, 503, map[string]string{"status": "unhealthy"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	r.Get("/api/v4/history", a.serveHistory)
	r.Get("/api/v4/contributors", a.serveHistory)
	r.Get("/api/v4/admin/athub/reviews", a.athubReviews)
	r.Get("/api/v4/admin/athub/reviews/{id}", a.athubReviews)
	r.Post("/api/v4/admin/athub/reviews/{id}", a.athubReviews)
	r.Get("/api/v4/account/athub/submissions", a.athubReviews)
	r.Post("/api/v4/account/athub/submissions", a.athubSubmit)
	r.Put("/api/v4/account/athub/novels/{id}", a.athubEditNovel)
	r.Get("/api/v4/account/athub/novels/{id}/illustrations", a.listNovelIllustrations)
	r.Post("/api/v4/account/athub/novels/{id}/illustrations", a.uploadNovelIllustration)
	r.Delete("/api/v4/account/athub/novels/{id}/illustrations/{illustrationId}", a.deleteNovelIllustration)
	r.Get("/api/v4/account/athub/novels/{id}/illustration-layout", a.readNovelIllustrationLayout)
	r.Put("/api/v4/account/athub/novels/{id}/illustration-layout", a.editNovelIllustrationLayout)
	r.Post("/api/v4/account/athub/cover", a.uploadNovelCover)
	r.Get("/api/v4/account/athub/submissions/{id}", a.athubReviews)
	return r
}

func (a *API) SetStorage(st interface {
	store.ATHubReviewStore
	Ping(context.Context) error
}) {
	a.reviews = st
	a.creatorPing = st.Ping
}

func (a *API) SetHistoryHandler(handler http.Handler) { a.history = handler }

func (a *API) serveHistory(w http.ResponseWriter, r *http.Request) {
	if a.history == nil {
		writeJSON(w, 503, map[string]string{"error": "history_unavailable"})
		return
	}
	a.history.ServeHTTP(w, r)
}
