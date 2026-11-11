package storage

import (
	"context"
	"time"
)

func (p *Postgres) GetTOC(ctx context.Context, novelID int32) (TOC, error) {
	// One snapshot includes empty volumes and ungrouped chapters without reading content.
	rows, err := p.db(ctx).Query(ctx, `SELECT t.kind, t."id", t."volumeId", t."order", t."title", t."createdAt", t."updatedAt"
		FROM "Novel" n LEFT JOIN LATERAL (
			SELECT 0 AS kind, "id", NULL::integer AS "volumeId", "order", "title", "createdAt", "updatedAt"
			FROM "Volume" WHERE "novelId" = n."id"
			UNION ALL
			SELECT 1 AS kind, "id", "volumeId", "order", "title", NULL::timestamptz AS "createdAt", "updatedAt"
			FROM "Chapter" WHERE "novelId" = n."id"
		) t ON true WHERE n."id" = $1 ORDER BY t.kind, t."order"`, novelID)
	if err != nil {
		return TOC{}, databaseError(err)
	}
	defer rows.Close()
	result := TOC{NovelID: novelID, Volumes: []Volume{}, Chapters: []ChapterSummary{}}
	foundNovel := false
	for rows.Next() {
		foundNovel = true
		var kind, id, volumeID, order *int32
		var title *string
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(&kind, &id, &volumeID, &order, &title, &createdAt, &updatedAt); err != nil {
			return TOC{}, err
		}
		// An existing novel with no volumes or chapters produces one all-null row.
		if id == nil {
			continue
		}
		if *kind == 0 {
			result.Volumes = append(result.Volumes, Volume{ID: *id,
				VolumeInput: VolumeInput{NovelID: novelID, Order: *order, Title: *title},
				CreatedAt:   *createdAt, UpdatedAt: *updatedAt})
		} else {
			result.Chapters = append(result.Chapters, ChapterSummary{ID: *id, NovelID: novelID,
				VolumeID: volumeID, Order: *order, Title: *title, UpdatedAt: *updatedAt})
		}
	}
	if err := rows.Err(); err != nil {
		return TOC{}, err
	}
	if !foundNovel {
		return TOC{}, ErrNotFound
	}
	return result, nil
}
