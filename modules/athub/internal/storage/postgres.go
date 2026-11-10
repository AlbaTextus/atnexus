package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

const novelColumns = `"id", "platformId", "authorId", "title", "summary", "coverUrl", "status"::text, "createdAt", "updatedAt"`
const chapterColumns = `"id", "novelId", "volumeId", "order", "title", "content", "updatedAt"`
const saveChapterSQL = `INSERT INTO "Chapter" ("novelId", "volumeId", "order", "title", "content", "updatedAt")
	VALUES ($1, $2, $3, $4, $5, now()) ON CONFLICT ("novelId", "order") DO UPDATE SET
	"volumeId" = EXCLUDED."volumeId", "title" = EXCLUDED."title", "content" = EXCLUDED."content", "updatedAt" = now()`

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanNovel(row pgx.Row) (Novel, error) {
	var n Novel
	err := row.Scan(&n.ID, &n.PlatformID, &n.AuthorID, &n.Title, &n.Summary, &n.CoverURL, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	return n, databaseError(err)
}

func scanChapter(row pgx.Row) (Chapter, error) {
	var c Chapter
	err := row.Scan(&c.ID, &c.NovelID, &c.VolumeID, &c.Order, &c.Title, &c.Content, &c.UpdatedAt)
	return c, databaseError(err)
}

func databaseError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return ErrInvalidReference
		case "23505":
			return ErrConflict
		}
	}
	return err
}

func (p *Postgres) GetNovel(ctx context.Context, id int32) (Novel, error) {
	return scanNovel(p.pool.QueryRow(ctx, `SELECT `+novelColumns+` FROM "Novel" WHERE "id" = $1`, id))
}

func (p *Postgres) ListNovels(ctx context.Context, f NovelFilter) ([]Novel, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+novelColumns+` FROM "Novel"
		WHERE ($1::integer = 0 OR "platformId" = $1) AND ($2::integer = 0 OR "authorId" = $2)
		ORDER BY "id" DESC LIMIT $3 OFFSET $4`, f.PlatformID, f.AuthorID, f.Limit, f.Offset)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]Novel, 0)
	for rows.Next() {
		n, err := scanNovel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// Prisma's @updatedAt runs only through Prisma; Go must write these timestamps.
func (p *Postgres) CreateNovel(ctx context.Context, n NovelInput) (Novel, error) {
	return createNovel(ctx, p.pool, n)
}

func createNovel(ctx context.Context, q queryRower, n NovelInput) (Novel, error) {
	result, err := scanNovel(q.QueryRow(ctx, `INSERT INTO "Novel"
		("platformId", "authorId", "title", "summary", "coverUrl", "status", "createdAt", "updatedAt")
		SELECT $1, $2, $3, $4, $5, $6::"NovelStatus", now(), now()
		WHERE $2::integer IS NULL OR EXISTS (SELECT 1 FROM "Author" WHERE "id" = $2 AND "platformId" = $1)
		RETURNING `+novelColumns, n.PlatformID, n.AuthorID, n.Title, n.Summary, n.CoverURL, n.Status))
	if errors.Is(err, ErrNotFound) {
		return Novel{}, ErrInvalidReference
	}
	return result, err
}

func (p *Postgres) UpdateNovel(ctx context.Context, id int32, n NovelInput) (Novel, error) {
	// Lock the novel so existence checks and updates cannot race with deletion.
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Novel{}, err
	}
	defer tx.Rollback(ctx)
	var existingID int32
	if err := tx.QueryRow(ctx, `SELECT "id" FROM "Novel" WHERE "id" = $1 FOR UPDATE`, id).Scan(&existingID); err != nil {
		return Novel{}, databaseError(err)
	}
	result, err := scanNovel(tx.QueryRow(ctx, `UPDATE "Novel" SET
		"platformId" = $2, "authorId" = $3, "title" = $4, "summary" = $5, "coverUrl" = $6,
		"status" = $7::"NovelStatus", "updatedAt" = now()
		WHERE "id" = $1 AND ($3::integer IS NULL OR EXISTS
			(SELECT 1 FROM "Author" WHERE "id" = $3 AND "platformId" = $2))
		RETURNING `+novelColumns, id, n.PlatformID, n.AuthorID, n.Title, n.Summary, n.CoverURL, n.Status))
	if errors.Is(err, ErrNotFound) {
		return Novel{}, ErrInvalidReference
	}
	if err != nil {
		return Novel{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Novel{}, databaseError(err)
	}
	return result, nil
}

func (p *Postgres) DeleteNovel(ctx context.Context, id int32) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM "Novel" WHERE "id" = $1`, id)
	return deleted(tag, err)
}

func (p *Postgres) GetChapter(ctx context.Context, novelID, order int32) (Chapter, error) {
	return scanChapter(p.pool.QueryRow(ctx, `SELECT `+chapterColumns+` FROM "Chapter" WHERE "novelId" = $1 AND "order" = $2`, novelID, order))
}

func (p *Postgres) ListChapters(ctx context.Context, novelID, limit, offset int32) ([]Chapter, error) {
	// A single statement distinguishes an absent novel from an empty chapter list.
	rows, err := p.pool.Query(ctx, `SELECT c."id", c."novelId", c."volumeId", c."order", c."title", c."content", c."updatedAt"
		FROM "Novel" n LEFT JOIN LATERAL
		(SELECT * FROM "Chapter" WHERE "novelId" = n."id" ORDER BY "order" LIMIT $2 OFFSET $3) c ON true
		WHERE n."id" = $1 ORDER BY c."order"`, novelID, limit, offset)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]Chapter, 0)
	foundNovel := false
	for rows.Next() {
		foundNovel = true
		var id *int32
		var novelID, volumeID, order *int32
		var title, content *string
		// The outer join emits one null chapter when the novel exists but the page is empty.
		var timestamp *time.Time
		if err := rows.Scan(&id, &novelID, &volumeID, &order, &title, &content, &timestamp); err != nil {
			return nil, err
		}
		if id != nil {
			result = append(result, Chapter{ID: *id, ChapterInput: ChapterInput{NovelID: *novelID, VolumeID: volumeID, Order: *order, Title: *title, Content: *content}, UpdatedAt: *timestamp})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !foundNovel {
		return nil, ErrNotFound
	}
	return result, nil
}

func (p *Postgres) SaveChapter(ctx context.Context, c ChapterInput) (Chapter, error) {
	return scanChapter(p.pool.QueryRow(ctx, saveChapterSQL+` RETURNING `+chapterColumns,
		c.NovelID, c.VolumeID, c.Order, c.Title, c.Content))
}

func (p *Postgres) DeleteChapter(ctx context.Context, novelID, order int32) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM "Chapter" WHERE "novelId" = $1 AND "order" = $2`, novelID, order)
	return deleted(tag, err)
}

func deleted(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) CheckSchema(ctx context.Context) error {
	// Fail at startup rather than silently creating or migrating Prisma-owned tables.
	for _, query := range []string{
		`SELECT "id", "platformId" FROM "Author" LIMIT 0`,
		`SELECT "id", "name", "url" FROM "Platform" LIMIT 0`,
		`SELECT ` + novelColumns + ` FROM "Novel" LIMIT 0`,
		`SELECT "id", "novelId", "order", "title", "createdAt", "updatedAt" FROM "Volume" LIMIT 0`,
		`SELECT ` + chapterColumns + ` FROM "Chapter" LIMIT 0`,
	} {
		rows, err := p.pool.Query(ctx, query)
		if err != nil {
			return fmt.Errorf("Prisma schema is not ready: %w", err)
		}
		rows.Close()
	}
	return nil
}
