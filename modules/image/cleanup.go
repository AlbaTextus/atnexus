package image

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/jackc/pgx/v5"
)

// PurgeExpiredUploads removes abandoned chunked uploads: their part files and
// then their session rows. It returns how many sessions were removed. Sessions
// locked by a request in progress are skipped and picked up by a later run.
func (m *Module) PurgeExpiredUploads(ctx context.Context) (int, error) {
	removed := 0
	for {
		done, err := m.purgeExpiredUpload(ctx)
		if err != nil || done {
			return removed, err
		}
		removed++
	}
}

func (m *Module) purgeExpiredUpload(ctx context.Context) (bool, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var id string
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT id,data FROM image_uploads WHERE expires_at<now() ORDER BY expires_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var session chunkSession
	if err = json.Unmarshal(raw, &session); err != nil {
		return false, err
	}
	// Files go first: a failure keeps the row, so the parts stay discoverable.
	for n := 0; n < session.Total; n++ {
		if err = m.scratch.Remove(chunkPartName(id, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM image_uploads WHERE id=$1`, id); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}
