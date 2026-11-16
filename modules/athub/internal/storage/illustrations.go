package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type IllustrationInput struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type Illustration struct {
	ID      int32 `json:"id"`
	NovelID int32 `json:"novelId"`
	IllustrationInput
	CreatedAt time.Time `json:"createdAt"`
}

const illustrationColumns = `id,"novelId",url,sha256,width,height,"createdAt"`

func scanIllustration(row pgx.Row) (Illustration, error) {
	var item Illustration
	err := row.Scan(&item.ID, &item.NovelID, &item.URL, &item.SHA256, &item.Width, &item.Height, &item.CreatedAt)
	return item, databaseError(err)
}

func (p *Postgres) ListIllustrations(ctx context.Context, novelID, limit, offset int32) ([]Illustration, error) {
	if _, err := p.GetNovel(ctx, novelID); err != nil {
		return nil, err
	}
	rows, err := p.db(ctx).Query(ctx, `SELECT `+illustrationColumns+` FROM "Illustration" WHERE "novelId"=$1 ORDER BY id LIMIT $2 OFFSET $3`, novelID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Illustration{}
	for rows.Next() {
		item, err := scanIllustration(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) AddIllustration(ctx context.Context, novelID int32, input IllustrationInput) (Illustration, error) {
	if err := p.trackNovel(ctx, novelID, false); err != nil {
		return Illustration{}, err
	}
	tx, err := p.db(ctx).Begin(ctx)
	if err != nil {
		return Illustration{}, err
	}
	defer tx.Rollback(ctx)
	var id int32
	if err := tx.QueryRow(ctx, `SELECT id FROM "Novel" WHERE id=$1 FOR UPDATE`, novelID).Scan(&id); err != nil {
		return Illustration{}, databaseError(err)
	}
	// A retry of the same image must not create another illustration or revision.
	item, err := scanIllustration(tx.QueryRow(ctx, `INSERT INTO "Illustration" ("novelId",url,sha256,width,height)
		VALUES($1,$2,$3,$4,$5) ON CONFLICT ("novelId",sha256) DO NOTHING RETURNING `+illustrationColumns,
		novelID, input.URL, input.SHA256, input.Width, input.Height))
	if err == ErrNotFound {
		item, err = scanIllustration(tx.QueryRow(ctx, `SELECT `+illustrationColumns+` FROM "Illustration" WHERE "novelId"=$1 AND sha256=$2`, novelID, input.SHA256))
	}
	if err != nil {
		return Illustration{}, err
	}
	return item, tx.Commit(ctx)
}

func (p *Postgres) DeleteIllustration(ctx context.Context, novelID, imageID int32) error {
	if err := p.trackNovel(ctx, novelID, false); err != nil {
		return err
	}
	tx, err := p.db(ctx).Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var locked int32
	if err := tx.QueryRow(ctx, `SELECT id FROM "Novel" WHERE id=$1 FOR UPDATE`, novelID).Scan(&locked); err != nil {
		return databaseError(err)
	}
	result, err := tx.Exec(ctx, `DELETE FROM "Illustration" WHERE id=$1 AND "novelId"=$2`, imageID, novelID)
	if err != nil {
		return databaseError(err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE "IllustrationLayout" SET
		blocks=COALESCE((SELECT jsonb_agg(b.value ORDER BY b.ordinality) FROM jsonb_array_elements(blocks) WITH ORDINALITY b
			WHERE (b.value->>'imageId')::int<>$2),'[]'::jsonb),revision=revision+1
		WHERE "novelId"=$1 AND blocks @> jsonb_build_array(jsonb_build_object('imageId',$2::int))`, novelID, imageID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
