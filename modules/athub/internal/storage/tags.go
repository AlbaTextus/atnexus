package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type TagInput struct {
	Name string `json:"name"`
}

type Tag struct {
	ID int32 `json:"id"`
	TagInput
}

func scanTag(row pgx.Row) (Tag, error) {
	var tag Tag
	err := row.Scan(&tag.ID, &tag.Name)
	return tag, databaseError(err)
}

func (p *Postgres) GetTag(ctx context.Context, id int32) (Tag, error) {
	return scanTag(p.db(ctx).QueryRow(ctx, `SELECT "id", "name" FROM "Tag" WHERE "id" = $1`, id))
}

func (p *Postgres) ListTags(ctx context.Context, limit, offset int32) ([]Tag, error) {
	rows, err := p.db(ctx).Query(ctx, `SELECT "id", "name" FROM "Tag" ORDER BY "id" LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]Tag, 0)
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tag)
	}
	return result, databaseError(rows.Err())
}

func (p *Postgres) CreateTag(ctx context.Context, input TagInput) (Tag, error) {
	return scanTag(p.db(ctx).QueryRow(ctx, `INSERT INTO "Tag" ("name") VALUES ($1) RETURNING "id", "name"`, input.Name))
}

func (p *Postgres) UpdateTag(ctx context.Context, id int32, input TagInput) (Tag, error) {
	if err := p.trackRelated(ctx, `SELECT "novelId" FROM "NovelTag" WHERE "tagId"=$1 ORDER BY "novelId"`, id); err != nil {
		return Tag{}, err
	}
	return scanTag(p.db(ctx).QueryRow(ctx, `UPDATE "Tag" SET "name" = $2 WHERE "id" = $1 RETURNING "id", "name"`, id, input.Name))
}

func (p *Postgres) DeleteTag(ctx context.Context, id int32) error {
	result, err := p.db(ctx).Exec(ctx, `DELETE FROM "Tag" WHERE "id" = $1`, id)
	if errors.Is(databaseError(err), ErrInvalidReference) {
		return ErrTagInUse
	}
	return deleted(result, err)
}

// The caller holds the novel lock and transaction so metadata and references
// are replaced atomically. Foreign keys reject missing tags.
func replaceNovelTags(ctx context.Context, tx pgx.Tx, novelID int32, ids []int32) ([]int32, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM "NovelTag" WHERE "novelId" = $1`, novelID); err != nil {
		return nil, databaseError(err)
	}
	if len(ids) > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO "NovelTag" ("novelId", "tagId")
			SELECT $1, id FROM unnest($2::integer[]) AS id ORDER BY id ON CONFLICT DO NOTHING`, novelID, ids); err != nil {
			return nil, databaseError(err)
		}
	}
	var result []int32
	err := tx.QueryRow(ctx, `SELECT ARRAY(SELECT "tagId" FROM "NovelTag" WHERE "novelId" = $1 ORDER BY "tagId")`, novelID).Scan(&result)
	return result, databaseError(err)
}
