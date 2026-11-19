// Package httpapi is the mail module's HTTP API under /api/v3.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/apibase"
	"github.com/AlbaTextus/atnexus/internal/config"
	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/mail"
	"github.com/go-chi/chi/v5"
)

var writeJSON = apibase.WriteJSON
var readAccountJSON = apibase.ReadJSON

// Account is the account module in this process. Mail applies its site rules
// and account view instead of keeping a second copy of them.
type Account interface {
	// RequireRegistration and VerifyHuman write the refusal themselves.
	RequireRegistration(http.ResponseWriter, *http.Request) bool
	VerifyHuman(http.ResponseWriter, *http.Request) bool
	MaintenanceGate(http.Handler) http.Handler
	UserView(store.User) map[string]any
}

type API struct {
	*apibase.Base
	account Account
	mailer  mail.Sender
}

func New(cfg config.Config, st store.Store, logger *slog.Logger, account Account) *API {
	if logger == nil {
		logger = slog.Default()
	}
	return &API{Base: &apibase.Base{Config: cfg, Store: st, Log: logger}, account: account, mailer: mail.New(cfg.SMTP)}
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	// Health stays reachable during maintenance and with SMTP left unconfigured.
	r.Get("/api/v3/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Store.Ping(r.Context()); err != nil {
			writeJSON(w, 503, map[string]string{"status": "unhealthy"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	r.Group(func(r chi.Router) {
		r.Use(a.account.MaintenanceGate)
		r.Post("/api/v3/auth/email/code", a.sendRegistrationCode)
		r.Post("/api/v3/account/email/code", a.sendAccountEmailCode)
		r.Post("/api/v3/account/email", a.confirmAccountEmail)
	})
	return r
}
