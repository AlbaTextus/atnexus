package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const volumeColumns = `"id", "novelId", "order", "title", "createdAt", "updatedAt"`
const saveVolumeSQL = `INSERT INTO "Volume" ("novelId", "order", "title", "createdAt", "updatedAt")
	VALUES ($1, $2, $3, now(), now()) ON CONFLICT ("novelId", "order") DO UPDATE SET
	"title" = EXCLUDED."title", "updatedAt" = now() RETURNING ` + volumeColumns

func scanVolume(row pgx.Row) (Volume, error) {
	var v Volume
	err := row.Scan(&v.ID, &v.NovelID, &v.Order, &v.Title, &v.CreatedAt, &v.UpdatedAt)
	return v, databaseError(err)
}

func (p *Postgres) GetVolume(ctx context.Context, novelID, order int32) (Volume, error) {
	return scanVolume(p.db(ctx).QueryRow(ctx, `SELECT `+volumeColumns+` FROM "Volume" WHERE "novelId" = $1 AND "order" = $2`, novelID, order))
}

func (p *Postgres) ListVolumes(ctx context.Context, novelID, limit, offset int32) ([]Volume, error) {
	// The outer join distinguishes an absent novel from an empty page.
	rows, err := p.db(ctx).Query(ctx, `SELECT c."id", c."novelId", c."order", c."title", c."createdAt", c."updatedAt"
		FROM "Novel" n LEFT JOIN LATERAL
		(SELECT * FROM "Volume" WHERE "novelId" = n."id" ORDER BY "order" LIMIT $2 OFFSET $3) c ON true
		WHERE n."id" = $1 ORDER BY c."order"`, novelID, limit, offset)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]Volume, 0)
	foundNovel := false
	for rows.Next() {
		foundNovel = true
		var id *int32
		var novelID, order *int32
		var title *string
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(&id, &novelID, &order, &title, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if id != nil {
			result = append(result, Volume{ID: *id, VolumeInput: VolumeInput{NovelID: *novelID, Order: *order, Title: *title}, CreatedAt: *createdAt, UpdatedAt: *updatedAt})
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

func (p *Postgres) SaveVolume(ctx context.Context, input VolumeInput) (Volume, error) {
	if err := p.trackNovel(ctx, input.NovelID, false); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Volume{}, ErrInvalidReference
		}
		return Volume{}, err
	}
	return scanVolume(p.db(ctx).QueryRow(ctx, saveVolumeSQL, input.NovelID, input.Order, input.Title))
}

func (p *Postgres) DeleteVolume(ctx context.Context, novelID, order int32) error {
	if err := p.trackNovel(ctx, novelID, false); err != nil {
		return err
	}
	tag, err := p.db(ctx).Exec(ctx, `DELETE FROM "Volume" WHERE "novelId" = $1 AND "order" = $2`, novelID, order)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrVolumeNotEmpty
	}
	return deleted(tag, err)
}
