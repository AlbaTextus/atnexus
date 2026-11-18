// Transitional account-database adapter. History business and SQL are owned
// by the creator platform; existing callers retain their persistence contract.
package store

import (
	"context"
	"encoding/json"

	"github.com/AlbaTextus/atnexus/modules/creatplat/repository"
	"github.com/google/uuid"
)

type NovelRevision = repository.NovelRevision
type NovelHistory = repository.NovelHistory
type NovelHistoryStore = repository.NovelHistoryStore

func (p *Postgres) CheckNovelHistorySchema(ctx context.Context) error {
	return repository.NewHistory(p.pool, ErrNotFound).CheckNovelHistorySchema(ctx)
}
func (p *Postgres) PrepareNovelRevisions(ctx context.Context, batch uuid.UUID, revisions []NovelRevision) error {
	return repository.NewHistory(p.pool, ErrNotFound).PrepareNovelRevisions(ctx, batch, revisions)
}
func (p *Postgres) FinishNovelRevisions(ctx context.Context, batch uuid.UUID, committed bool) error {
	return repository.NewHistory(p.pool, ErrNotFound).FinishNovelRevisions(ctx, batch, committed)
}
func (p *Postgres) NovelHistory(ctx context.Context, id int32, limit, offset int) (NovelHistory, error) {
	return repository.NewHistory(p.pool, ErrNotFound).NovelHistory(ctx, id, limit, offset)
}
func (p *Postgres) NovelContributors(ctx context.Context, id int32) (json.RawMessage, error) {
	return repository.NewHistory(p.pool, ErrNotFound).NovelContributors(ctx, id)
}
func (p *Postgres) NovelRevisionChanges(ctx context.Context, id int32, revision uuid.UUID, limit, offset int) (repository.RevisionChanges, error) {
	return repository.NewHistory(p.pool, ErrNotFound).NovelRevisionChanges(ctx, id, revision, limit, offset)
}
