package image

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	janitorInterval = 10 * time.Minute
	janitorBatch    = 200
	// Interrupted deletions are retried once no request can still own them.
	staleDeleting = 15 * time.Minute
	// Reservations and scratch files are only reconciled long after any upload
	// that could still be writing them has been cut off by the HTTP timeout.
	stalePending = 6 * time.Hour
	staleScratch = 24 * time.Hour
)

// janitor reclaims what interrupted requests leave behind: expired chunk
// sessions and their part files, rows stuck in deleting or pending, and
// abandoned scratch files. Every step is idempotent and safe to repeat.
func (m *Module) janitor(ctx context.Context) {
	defer close(m.done)
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()
	for {
		m.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Module) sweep(ctx context.Context) {
	for name, step := range map[string]func(context.Context) error{
		"expired upload sessions": m.sweepUploads,
		"interrupted deletions":   m.sweepDeleting,
		"unfinished reservations": m.sweepPending,
		"scratch files":           m.sweepScratch,
	} {
		if ctx.Err() != nil {
			return
		}
		if err := step(ctx); err != nil && ctx.Err() == nil {
			m.deps.Logger.Error("image cleanup failed", "step", name, "error", safeError(err))
		}
	}
}

func (m *Module) sweepUploads(ctx context.Context) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	// SKIP LOCKED leaves sessions alone while a request is still using them.
	rows, err := tx.Query(ctx, `SELECT id,data FROM image_uploads WHERE expires_at<now() ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED`, janitorBatch)
	if err != nil {
		return err
	}
	var ids []string
	var totals []int
	for rows.Next() {
		var id string
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var session chunkSession
		_ = decodeJSON(raw, &session)
		ids = append(ids, id)
		totals = append(totals, session.Total)
	}
	rows.Close()
	if err = rows.Err(); err != nil || len(ids) == 0 {
		return err
	}
	// One part that cannot be removed must not hold back the rest of the batch:
	// that session is left for the next sweep and the others are still reclaimed.
	removable := ids[:0]
	for i, id := range ids {
		var failed error
		for n := 0; n < totals[i] && n < m.cfg.MaxChunks; n++ {
			if err = m.scratch.Remove(chunkPartName(id, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
				failed = err
			}
		}
		if failed != nil {
			m.deps.Logger.Error("image cleanup could not remove an upload part", "error", safeError(failed))
			continue
		}
		removable = append(removable, id)
	}
	if len(removable) == 0 {
		return nil
	}
	if _, err = tx.Exec(ctx, `DELETE FROM image_uploads WHERE id=ANY($1)`, removable); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *Module) sweepDeleting(ctx context.Context) error {
	ids, err := m.staleIDs(ctx, "deleting", staleDeleting)
	if err != nil {
		return err
	}
	for _, id := range ids {
		// One unreachable object must not block the rest of the batch.
		if err = m.deleteFile(ctx, id); err != nil && !errors.Is(err, ErrNotFound) && ctx.Err() == nil {
			m.deps.Logger.Error("image cleanup could not finish a deletion", "error", safeError(err))
		}
	}
	return nil
}

// sweepPending settles reservations whose request ended between storing the
// bytes and marking the row ready: complete objects become ready, reservations
// without bytes are released so the name and quota are free again.
func (m *Module) sweepPending(ctx context.Context) error {
	ids, err := m.staleIDs(ctx, "pending", stalePending)
	if err != nil {
		return err
	}
	for _, id := range ids {
		f, err := scanFile(m.pool.QueryRow(ctx, `SELECT `+fileColumns+` FROM image_files WHERE id=$1 AND state='pending'`, id))
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		store, key, err := m.storeFor(ctx, f.Metadata)
		if err != nil {
			// The channel was removed or renamed; leave the row for an operator.
			continue
		}
		if key == "" {
			key = id
		}
		info, err := store.Open(ctx, key, "", true)
		if info.Body != nil {
			info.Body.Close()
		}
		var remote *smithyhttp.ResponseError
		switch {
		case err == nil && sizeMatches(f.Metadata, info.Size):
			_, err = m.pool.Exec(ctx, `UPDATE image_files SET state='ready',updated_at=now() WHERE id=$1 AND state='pending'`, id)
		case errors.Is(err, os.ErrNotExist), errors.As(err, &remote) && remote.HTTPStatusCode() == 404:
			_, err = m.pool.Exec(ctx, `DELETE FROM image_files WHERE id=$1 AND state='pending'`, id)
		case err == nil:
			// An object of another size is not this upload; keep it for review.
			continue
		}
		if err != nil && ctx.Err() == nil {
			m.deps.Logger.Error("image cleanup could not settle a reservation", "error", safeError(err))
		}
	}
	return nil
}

func sizeMatches(metadata map[string]any, size int64) bool {
	number, ok := metadata["FileSizeBytes"].(interface{ Int64() (int64, error) })
	if !ok {
		return false
	}
	expected, err := number.Int64()
	return err == nil && expected == size
}

func (m *Module) staleIDs(ctx context.Context, state string, age time.Duration) ([]string, error) {
	rows, err := m.pool.Query(ctx, `SELECT id FROM image_files WHERE state=$1 AND updated_at<$2 ORDER BY updated_at LIMIT $3`, state, time.Now().Add(-age), janitorBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// sweepScratch removes request and merge spool files that outlived any request
// that could own them, which only happens when the process died mid-request.
func (m *Module) sweepScratch(ctx context.Context) error {
	dir, err := m.scratch.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	cutoff := time.Now().Add(-staleScratch)
	for ctx.Err() == nil {
		entries, err := dir.ReadDir(256)
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, "request-") && !strings.HasPrefix(name, "merge-") {
				continue
			}
			if info, e := entry.Info(); e == nil && info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
				_ = m.scratch.Remove(name)
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
