package storage

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

type IllustrationBlock struct {
	ImageID int32   `json:"imageId"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
}

type IllustrationLayout struct {
	NovelID  int32               `json:"novelId"`
	Blocks   []IllustrationBlock `json:"blocks"`
	Revision int32               `json:"revision"`
}

func (p *Postgres) GetIllustrationLayout(ctx context.Context, novelID int32) (IllustrationLayout, error) {
	value := IllustrationLayout{NovelID: novelID, Blocks: []IllustrationBlock{}}
	if _, err := p.GetNovel(ctx, novelID); err != nil {
		return value, err
	}
	var raw []byte
	err := p.db(ctx).QueryRow(ctx, `SELECT blocks,revision FROM "IllustrationLayout" WHERE "novelId"=$1`, novelID).Scan(&raw, &value.Revision)
	if err == pgx.ErrNoRows {
		return value, nil
	}
	if err != nil {
		return value, databaseError(err)
	}
	if err := json.Unmarshal(raw, &value.Blocks); err != nil {
		return value, err
	}
	return value, nil
}

func (p *Postgres) SaveIllustrationLayout(ctx context.Context, novelID int32, input IllustrationLayout) (IllustrationLayout, error) {
	if err := p.trackNovel(ctx, novelID, false); err != nil {
		return IllustrationLayout{}, err
	}
	tx, err := p.db(ctx).Begin(ctx)
	if err != nil {
		return IllustrationLayout{}, err
	}
	defer tx.Rollback(ctx)
	var locked int32
	if err := tx.QueryRow(ctx, `SELECT id FROM "Novel" WHERE id=$1 FOR UPDATE`, novelID).Scan(&locked); err != nil {
		return IllustrationLayout{}, databaseError(err)
	}
	var revision int32
	err = tx.QueryRow(ctx, `SELECT revision FROM "IllustrationLayout" WHERE "novelId"=$1`, novelID).Scan(&revision)
	if err != nil && err != pgx.ErrNoRows {
		return IllustrationLayout{}, err
	}
	if revision != input.Revision {
		return IllustrationLayout{}, ErrConflict
	}
	ids := make([]int32, 0, len(input.Blocks))
	for _, block := range input.Blocks {
		ids = append(ids, block.ImageID)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM "Illustration" WHERE "novelId"=$1 AND id=ANY($2::int[])`, novelID, ids).Scan(&count); err != nil {
		return IllustrationLayout{}, err
	}
	if count != len(ids) {
		return IllustrationLayout{}, ErrInvalidReference
	}
	raw, err := json.Marshal(input.Blocks)
	if err != nil {
		return IllustrationLayout{}, err
	}
	input.NovelID = novelID
	if err := tx.QueryRow(ctx, `INSERT INTO "IllustrationLayout" ("novelId",blocks,revision) VALUES($1,$2,1)
		ON CONFLICT ("novelId") DO UPDATE SET blocks=EXCLUDED.blocks,revision="IllustrationLayout".revision+1
		RETURNING revision`, novelID, raw).Scan(&input.Revision); err != nil {
		return IllustrationLayout{}, err
	}
	return input, tx.Commit(ctx)
}
