// Package storage owns the creator platform's independent PostgreSQL database.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/creatplat/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReviewAuthority interface {
	WithCreatorSubmission(context.Context, store.ATHubReview, func() error) error
	WithCreatorReviewAccess(context.Context, uuid.UUID, func() error) error
	WithCreatorEditAccess(context.Context, uuid.UUID, func() error) error
	WithCreatorDecision(context.Context, uuid.UUID, store.ATHubReview, bool, func(json.RawMessage) error) error
}

type Postgres struct {
	*repository.History
	pool      *pgxpool.Pool
	authority ReviewAuthority
}

func New(ctx context.Context, databaseURL string, authority ReviewAuthority) (*Postgres, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("CREATPLAT_DATABASE_URL is required")
	}
	if authority == nil {
		return nil, errors.New("creator account authorization is required")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid CREATPLAT_DATABASE_URL")
	}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not create creator PostgreSQL pool")
	}
	st := &Postgres{pool: pool, authority: authority, History: repository.NewHistory(pool, store.ErrNotFound)}
	if err := st.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("creator database unavailable; check CREATPLAT_DATABASE_URL")
	}
	if err := st.CheckSchema(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("creator schema is not ready; run npm run db:creatplat:deploy: %w", err)
	}
	return st, nil
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
func (p *Postgres) Close()                         { p.pool.Close() }

func (p *Postgres) CheckSchema(ctx context.Context) error {
	for _, query := range []string{
		`SELECT id,user_id,contributor,operation,method,path,query,payload,status,reviewer_id,reviewer,note,result_status,result,created_at,reviewed_at,baseline FROM athub_reviews LIMIT 0`,
		`SELECT key,source_database,source_server,review_digest,history_digest,review_count,history_count,completed_at FROM creator_migration_state LIMIT 0`,
	} {
		rows, err := p.pool.Query(ctx, query)
		if err != nil {
			return err
		}
		rows.Close()
	}
	return p.CheckNovelHistorySchema(ctx)
}
