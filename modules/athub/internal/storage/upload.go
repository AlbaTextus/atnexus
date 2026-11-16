package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) Upload(ctx context.Context, input UploadInput) (UploadResult, error) {
	if input.NovelID != nil {
		if err := p.trackNovel(ctx, *input.NovelID, false); err != nil {
			return UploadResult{}, err
		}
	}
	result := UploadResult{Volumes: make([]Volume, 0), Chapters: make([]UploadedChapter, 0)}
	tx, err := p.db(ctx).Begin(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	defer tx.Rollback(ctx)
	if input.NovelID == nil {
		novel := *input.Novel
		if input.AuthorName != nil && novel.AuthorID == nil {
			// Resolve inside the upload transaction: failed uploads and pending
			// reviews must never leave an orphan author behind.
			id, err := uploadAuthor(ctx, tx, novel.PlatformID, *input.AuthorName)
			if err != nil {
				return UploadResult{}, err
			}
			novel.AuthorID = &id
		}
		result.Novel, err = createNovel(ctx, tx, novel)
	} else {
		// Serialize uploads to one novel and prevent deletion during the batch.
		result.Novel, err = scanNovel(tx.QueryRow(ctx, `SELECT `+novelReadColumns+` FROM "Novel" WHERE "id" = $1 FOR NO KEY UPDATE`, *input.NovelID))
	}
	if err != nil {
		return UploadResult{}, err
	}
	if input.NovelID == nil {
		if err := p.trackNovel(ctx, result.Novel.ID, true); err != nil {
			return UploadResult{}, err
		}
	}
	if len(input.Volumes) > 0 {
		result.Volumes, err = uploadVolumes(ctx, tx, result.Novel.ID, input.Volumes)
		if err != nil {
			return UploadResult{}, err
		}
	}
	volumeOrders := make([]int32, 0)
	for _, c := range input.Chapters {
		if c.VolumeOrder != nil {
			volumeOrders = append(volumeOrders, *c.VolumeOrder)
		}
	}
	volumeIDs := make(map[int32]int32)
	if len(volumeOrders) > 0 {
		rows, err := tx.Query(ctx, `SELECT "order", "id" FROM "Volume"
			WHERE "novelId" = $1 AND "order" = ANY($2::integer[]) FOR KEY SHARE`, result.Novel.ID, volumeOrders)
		if err != nil {
			return UploadResult{}, databaseError(err)
		}
		for rows.Next() {
			var order, id int32
			if err := rows.Scan(&order, &id); err != nil {
				rows.Close()
				return UploadResult{}, err
			}
			volumeIDs[order] = id
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return UploadResult{}, err
		}
	}
	batch := &pgx.Batch{}
	chapterOrders := make([]int32, 0, len(input.Chapters))
	for _, c := range input.Chapters {
		chapterOrders = append(chapterOrders, c.Order)
	}
	if err := p.trackChapterBodies(ctx, tx, result.Novel.ID, chapterOrders); err != nil {
		return UploadResult{}, err
	}
	for _, c := range input.Chapters {
		var volumeID *int32
		if c.VolumeOrder != nil {
			id, ok := volumeIDs[*c.VolumeOrder]
			if !ok {
				return UploadResult{}, ErrInvalidReference
			}
			volumeID = &id
		}
		batch.Queue(saveChapterSQL+` RETURNING "id", "order", "volumeId", "updatedAt"`, result.Novel.ID, volumeID, c.Order, c.Title, c.Content)
	}
	if len(input.Chapters) > 0 {
		result.Chapters, err = uploadChapters(ctx, tx, batch, len(input.Chapters))
		if err != nil {
			return UploadResult{}, err
		}
	}
	result.ChapterCount = len(result.Chapters)
	if err := tx.Commit(ctx); err != nil {
		return UploadResult{}, databaseError(err)
	}
	return result, nil
}

func uploadAuthor(ctx context.Context, tx pgx.Tx, platformID int32, name string) (int32, error) {
	// Serialize same-platform/name imports, including when no author exists yet.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2))`, platformID, name); err != nil {
		return 0, databaseError(err)
	}
	var id int32
	err := tx.QueryRow(ctx, `SELECT "id" FROM "Author" WHERE "platformId"=$1 AND btrim("name")=$2 ORDER BY "id" LIMIT 1 FOR KEY SHARE`, platformID, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO "Author" ("platformId", "name") VALUES ($1, $2) RETURNING "id"`, platformID, name).Scan(&id)
	}
	return id, databaseError(err)
}

func uploadVolumes(ctx context.Context, tx pgx.Tx, novelID int32, inputs []UploadVolume) ([]Volume, error) {
	batch := &pgx.Batch{}
	for _, v := range inputs {
		batch.Queue(saveVolumeSQL, novelID, v.Order, v.Title)
	}
	rows := tx.SendBatch(ctx, batch)
	defer rows.Close()
	result := make([]Volume, 0, len(inputs))
	for range inputs {
		v, err := scanVolume(rows.QueryRow())
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, databaseError(rows.Close())
}

func uploadChapters(ctx context.Context, tx pgx.Tx, batch *pgx.Batch, count int) ([]UploadedChapter, error) {
	rows := tx.SendBatch(ctx, batch)
	defer rows.Close()
	result := make([]UploadedChapter, 0, count)
	for range count {
		var c UploadedChapter
		if err := rows.QueryRow().Scan(&c.ID, &c.Order, &c.VolumeID, &c.UpdatedAt); err != nil {
			return nil, databaseError(err)
		}
		result = append(result, c)
	}
	return result, databaseError(rows.Close())
}
