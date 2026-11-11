package storage

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func scanPlatform(row pgx.Row) (Platform, error) {
	var p Platform
	err := row.Scan(&p.ID, &p.Name, &p.URL)
	return p, databaseError(err)
}

func (p *Postgres) GetPlatform(ctx context.Context, id int32) (Platform, error) {
	return scanPlatform(p.db(ctx).QueryRow(ctx, `SELECT "id", "name", "url" FROM "Platform" WHERE "id" = $1`, id))
}

func (p *Postgres) ListPlatforms(ctx context.Context, limit, offset int32) ([]Platform, error) {
	rows, err := p.db(ctx).Query(ctx, `SELECT "id", "name", "url" FROM "Platform" ORDER BY "id" LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]Platform, 0)
	for rows.Next() {
		item, err := scanPlatform(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (p *Postgres) CreatePlatform(ctx context.Context, input PlatformInput) (Platform, error) {
	return scanPlatform(p.db(ctx).QueryRow(ctx, `INSERT INTO "Platform" ("name", "url") VALUES ($1, $2)
		RETURNING "id", "name", "url"`, input.Name, input.URL))
}

func scanAuthor(row pgx.Row) (Author, error) {
	var a Author
	err := row.Scan(&a.ID, &a.PlatformID, &a.Name)
	return a, databaseError(err)
}

func (p *Postgres) GetAuthor(ctx context.Context, id int32) (Author, error) {
	return scanAuthor(p.db(ctx).QueryRow(ctx, `SELECT "id", "platformId", "name" FROM "Author" WHERE "id" = $1`, id))
}

func (p *Postgres) ListAuthors(ctx context.Context, platformID, limit, offset int32) ([]Author, error) {
	rows, err := p.db(ctx).Query(ctx, `SELECT "id", "platformId", "name" FROM "Author"
		WHERE ($1::integer = 0 OR "platformId" = $1) ORDER BY "id" LIMIT $2 OFFSET $3`, platformID, limit, offset)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]Author, 0)
	for rows.Next() {
		item, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (p *Postgres) CreateAuthor(ctx context.Context, input AuthorInput) (Author, error) {
	return scanAuthor(p.db(ctx).QueryRow(ctx, `INSERT INTO "Author" ("platformId", "name") VALUES ($1, $2)
		RETURNING "id", "platformId", "name"`, input.PlatformID, input.Name))
}

func (p *Postgres) UpdateAuthorName(ctx context.Context, id int32, name *string) (Author, error) {
	if err := p.trackRelated(ctx, `SELECT id FROM "Novel" WHERE "authorId"=$1 ORDER BY id`, id); err != nil {
		return Author{}, err
	}
	return scanAuthor(p.db(ctx).QueryRow(ctx, `UPDATE "Author" SET "name"=$2 WHERE "id"=$1
		RETURNING "id", "platformId", "name"`, id, name))
}
