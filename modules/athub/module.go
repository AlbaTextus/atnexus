// Package athub embeds the content API in a host process.
// It owns its database pool, but never opens an HTTP listener.
package athub

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/api"
	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	DatabaseURL   string
	WrapHandler   func(http.Handler) http.Handler
	RecordChanges ChangeRecorder
}

type Contributor = storage.Contributor
type RevisionDraft = storage.RevisionDraft
type Change = storage.Change
type ChangeRecorder = storage.ChangeRecorder

// ErrImageMissing is what a change recorder returns to refuse a change whose
// new cover or illustration is an image the image host no longer has.
var ErrImageMissing = storage.ErrImageMissing

// WithContributor is for host middleware, never for client-supplied identities.
func WithContributor(ctx context.Context, contributor Contributor) context.Context {
	return storage.WithContributor(ctx, contributor)
}

// WithMutationID is used only by the host's approved-submission executor.
func WithMutationID(ctx context.Context, id string) context.Context {
	return storage.WithMutationID(ctx, id)
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("ATHUB_DATABASE_URL is required")
	}
	if c.WrapHandler == nil {
		return errors.New("host ATHub authentication middleware is required")
	}
	if _, err := pgxpool.ParseConfig(c.DatabaseURL); err != nil {
		return errors.New("invalid ATHUB_DATABASE_URL")
	}
	return nil
}

type Module struct {
	pool    *pgxpool.Pool
	store   *storage.Postgres
	handler http.Handler
}

// New checks the existing schema. Migrations remain an explicit release step.
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, errors.New("invalid ATHUB_DATABASE_URL")
	}
	poolConfig.MaxConns = 10
	poolConfig.MinConns = 0
	poolConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(startup, poolConfig)
	if err != nil {
		return nil, errors.New("could not create ATHub PostgreSQL pool")
	}
	ready := false
	defer func() {
		if !ready {
			pool.Close()
		}
	}()
	if err := pool.Ping(startup); err != nil {
		return nil, errors.New("ATHub PostgreSQL connection failed; check ATHUB_DATABASE_URL and database availability")
	}
	st := storage.NewPostgres(pool)
	st.RecordChanges = cfg.RecordChanges
	if err := st.CheckSchema(startup); err != nil {
		return nil, errors.New("ATHub schema is not ready; run npm run db:athub:deploy")
	}
	handler, err := api.NewWithMiddleware(st, cfg.WrapHandler, logger)
	if err != nil {
		return nil, err
	}
	ready = true
	return &Module{pool: pool, store: st, handler: handler}, nil
}

func (m *Module) Handler() http.Handler          { return m.handler }
func (m *Module) Ping(ctx context.Context) error { return m.pool.Ping(ctx) }
func (m *Module) Close()                         { m.pool.Close() }

// Rehearsal receives what a rehearsed change would alter; see WithRehearsal.
type Rehearsal = storage.Rehearsal

// WithRehearsal makes the content handler run a change and roll it back, so the
// host can learn how it would be answered, and what it would alter, without
// storing anything.
func WithRehearsal(ctx context.Context) (context.Context, *Rehearsal) {
	return storage.WithRehearsal(ctx)
}

// WithBaseline is used only by the host's approved-submission executor. It
// hands back the baseline a rehearsal of the same change produced, so that the
// change is applied to what it altered then and refused when that has changed.
func WithBaseline(ctx context.Context, baseline []byte) context.Context {
	return storage.WithBaseline(ctx, baseline)
}

// Rehearsing reports whether ctx belongs to a rehearsal.
func Rehearsing(ctx context.Context) bool { return storage.Rehearsing(ctx) }

// ForgetMutation drops the replay receipt of a WithMutationID call. Call it only
// once the host has committed that mutation's final outcome.
func (m *Module) ForgetMutation(ctx context.Context, id string) error {
	return m.store.DeleteMutationReceipt(ctx, id)
}

// CommittedRevisions reports which revision batches reached the content
// database, for settling revisions whose completion was lost.
func (m *Module) CommittedRevisions(ctx context.Context, batches []string) ([]string, error) {
	return m.store.CommittedRevisions(ctx, batches)
}

// HoldImage is called by the change recorder for an image address a change
// starts to point at, before it checks that the image is still stored. The
// image cannot be removed through ReleaseImage until that change has ended.
func (m *Module) HoldImage(ctx context.Context, location string) error {
	return m.store.HoldImage(ctx, location)
}

// ReleaseImage calls remove unless a novel cover or illustration points to the
// given address, so the host can delete a stored image nothing needs any more.
func (m *Module) ReleaseImage(ctx context.Context, location string, remove func(context.Context) error) error {
	return m.store.ReleaseImage(ctx, location, remove)
}

// PruneRevisionReceipts drops receipts older than the given time. Call it only
// after every revision that old has been settled.
func (m *Module) PruneRevisionReceipts(ctx context.Context, before time.Time) error {
	return m.store.DeleteRevisionReceipts(ctx, before)
}
