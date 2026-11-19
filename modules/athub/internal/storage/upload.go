package storage

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
)

const uploadedChapterColumns = ` RETURNING "id", "order", "volumeId", "updatedAt"`

// chapterWrite is a chapter of an upload, with what it keeps of the chapter
// stored at its order when the upload is applied after a rehearsal.
type chapterWrite struct {
	UploadChapter
	// The chapter stays in volumeID, whatever VolumeOrder says.
	keepVolume bool
	volumeID   *int32
	keepBody   bool
	// The stored chapter, when nothing of it is written.
	stored *UploadedChapter
}

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
	volumes, chapters := input.Volumes, make([]chapterWrite, len(input.Chapters))
	for i, c := range input.Chapters {
		chapters[i].UploadChapter = c
	}
	if records, ok := submitted(ctx, result.Novel.ID); ok {
		if volumes, err = keepUpload(ctx, tx, result.Novel.ID, records, volumes, chapters); err != nil {
			return UploadResult{}, err
		}
	}
	if len(volumes) > 0 {
		result.Volumes, err = uploadVolumes(ctx, tx, result.Novel.ID, volumes)
		if err != nil {
			return UploadResult{}, err
		}
	}
	volumeOrders := make([]int32, 0)
	for _, c := range chapters {
		if c.VolumeOrder != nil && !c.keepVolume {
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
	chapterOrders := make([]int32, 0, len(chapters))
	for _, c := range chapters {
		if !c.keepBody {
			chapterOrders = append(chapterOrders, c.Order)
		}
	}
	if err := p.trackChapterBodies(ctx, tx, result.Novel.ID, chapterOrders); err != nil {
		return UploadResult{}, err
	}
	for _, c := range chapters {
		if c.stored != nil {
			continue
		}
		volumeID := c.volumeID
		if c.VolumeOrder != nil && !c.keepVolume {
			id, ok := volumeIDs[*c.VolumeOrder]
			if !ok {
				return UploadResult{}, ErrInvalidReference
			}
			volumeID = &id
		}
		if c.keepBody {
			batch.Queue(saveChapterHeadingSQL+uploadedChapterColumns, result.Novel.ID, volumeID, c.Order, c.Title)
		} else {
			batch.Queue(saveChapterSQL+uploadedChapterColumns, result.Novel.ID, volumeID, c.Order, c.Title, c.Content)
		}
	}
	var written []UploadedChapter
	if batch.Len() > 0 {
		if written, err = uploadChapters(ctx, tx, batch, batch.Len()); err != nil {
			return UploadResult{}, err
		}
	}
	for _, c := range chapters {
		if c.stored != nil {
			// Nothing of it was written, so it is reported as it is stored.
			result.Chapters = append(result.Chapters, *c.stored)
			continue
		}
		result.Chapters, written = append(result.Chapters, written[0]), written[1:]
	}
	result.ChapterCount = len(result.Chapters)
	if err := tx.Commit(ctx); err != nil {
		return UploadResult{}, databaseError(err)
	}
	return result, nil
}

// keepUpload prepares an upload into an existing novel that is applied after a
// rehearsal: volumes and chapters the rehearsal left alone keep what they hold
// now. It returns the volumes to save and marks on the chapters what is not to
// be written.
func keepUpload(ctx context.Context, tx pgx.Tx, novelID int32, records baselineRecords, volumes []UploadVolume, chapters []chapterWrite) ([]UploadVolume, error) {
	orders := make([]int32, 0, len(volumes))
	for _, v := range volumes {
		if records.keptVolume(v.Order) {
			orders = append(orders, v.Order)
		}
	}
	if len(orders) > 0 {
		rows, err := tx.Query(ctx, `SELECT "order", "title" FROM "Volume"
			WHERE "novelId" = $1 AND "order" = ANY($2::integer[])`, novelID, orders)
		if err != nil {
			return nil, databaseError(err)
		}
		titles := make(map[int32]string, len(orders))
		for rows.Next() {
			var order int32
			var title string
			if err := rows.Scan(&order, &title); err != nil {
				rows.Close()
				return nil, err
			}
			titles[order] = title
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		// The caller's volumes are not ours to rewrite.
		volumes = slices.Clone(volumes)
		for i := range volumes {
			if title, ok := titles[volumes[i].Order]; ok {
				volumes[i].Title = title
			}
		}
	}
	kept := make(map[int32]keptChapter, len(chapters))
	orders = make([]int32, 0, len(chapters))
	for _, c := range chapters {
		if parts, ok := records.keptChapter(c.Order); ok {
			kept[c.Order] = parts
			orders = append(orders, c.Order)
		}
	}
	if len(orders) == 0 {
		return volumes, nil
	}
	// Headings tell what stays as it is; no body is read for that.
	rows, err := tx.Query(ctx, `SELECT "id", "order", "volumeId", "updatedAt", "title" FROM "Chapter"
		WHERE "novelId" = $1 AND "order" = ANY($2::integer[])`, novelID, orders)
	if err != nil {
		return nil, databaseError(err)
	}
	stored := make(map[int32]UploadedChapter, len(orders))
	titles := make(map[int32]string, len(orders))
	for rows.Next() {
		var c UploadedChapter
		var title string
		if err := rows.Scan(&c.ID, &c.Order, &c.VolumeID, &c.UpdatedAt, &title); err != nil {
			rows.Close()
			return nil, err
		}
		stored[c.Order], titles[c.Order] = c, title
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range chapters {
		c := &chapters[i]
		current, exists := stored[c.Order]
		if !exists {
			// Nothing is stored at this order, or the rehearsal replaced what
			// was: the chapter is written as it was submitted.
			continue
		}
		parts := kept[c.Order]
		if parts.title {
			c.Title = titles[c.Order]
		}
		if parts.volume {
			c.keepVolume, c.volumeID = true, current.VolumeID
		}
		c.keepBody = parts.body
		if parts.title && parts.volume && parts.body {
			c.stored = &current
		}
	}
	return volumes, nil
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
