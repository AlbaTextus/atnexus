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

	"albatextus/athub/internal/api"
	"albatextus/athub/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	DatabaseURL string
	WrapHandler func(http.Handler) http.Handler
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
	if err := st.CheckSchema(startup); err != nil {
		return nil, errors.New("ATHub schema is not ready; run npm run db:athub:deploy")
	}
	handler, err := api.NewWithMiddleware(st, cfg.WrapHandler, logger)
	if err != nil {
		return nil, err
	}
	ready = true
	return &Module{pool: pool, handler: handler}, nil
}

func (m *Module) Handler() http.Handler          { return m.handler }
func (m *Module) Ping(ctx context.Context) error { return m.pool.Ping(ctx) }
func (m *Module) Close()                         { m.pool.Close() }
