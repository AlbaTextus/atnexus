package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type databaseIdentity struct{ Database, Server string }

func identify(ctx context.Context, pool *pgxpool.Pool) (databaseIdentity, error) {
	var id databaseIdentity
	err := pool.QueryRow(ctx, `SELECT current_database(),COALESCE(inet_server_addr()::text,'local')||':'||COALESCE(inet_server_port()::text,'socket')`).Scan(&id.Database, &id.Server)
	return id, err
}

// RequireSeparateDatabases rejects a second schema or credential pointing at
// the same physical database. Creator data cannot silently use an account pool.
func (p *Postgres) RequireSeparateDatabases(ctx context.Context, urls ...string) error {
	current, err := identify(ctx, p.pool)
	if err != nil {
		return err
	}
	for _, raw := range urls {
		other, err := pgxpool.New(ctx, raw)
		if err != nil {
			return errors.New("could not check database isolation")
		}
		id, err := identify(ctx, other)
		other.Close()
		if err != nil {
			return errors.New("could not check database isolation")
		}
		if current == id {
			return errors.New("CREATPLAT_DATABASE_URL must identify a database separate from OAuth and ATHub")
		}
	}
	return nil
}

type tableDigest struct {
	Sum   string
	Count int64
}

func digest(ctx context.Context, tx pgx.Tx, table string) (tableDigest, error) {
	rows, err := tx.Query(ctx, `SELECT to_jsonb(t)::text FROM `+pgx.Identifier{table}.Sanitize()+` t ORDER BY id`)
	if err != nil {
		return tableDigest{}, err
	}
	defer rows.Close()
	hash := sha256.New()
	var count int64
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return tableDigest{}, err
		}
		_, _ = io.WriteString(hash, raw)
		_, _ = io.WriteString(hash, "\n")
		count++
	}
	if err := rows.Err(); err != nil {
		return tableDigest{}, err
	}
	return tableDigest{Sum: hex.EncodeToString(hash.Sum(nil)), Count: count}, nil
}

func sourceSnapshot(ctx context.Context, pool *pgxpool.Pool) (tableDigest, tableDigest, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return tableDigest{}, tableDigest{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL TIME ZONE 'UTC'`); err != nil {
		return tableDigest{}, tableDigest{}, err
	}
	reviews, err := digest(ctx, tx, "athub_reviews")
	if err != nil {
		return reviews, tableDigest{}, err
	}
	history, err := digest(ctx, tx, "novel_revisions")
	if err != nil {
		return reviews, history, err
	}
	return reviews, history, tx.Commit(ctx)
}

// ErrLegacyDataMismatch means the retired source tables do not match what was
// imported. Other check failures (timeouts, connectivity) are not this error.
var ErrLegacyDataMismatch = errors.New("legacy creator data mismatch")

// CheckLegacyMigration refuses to start against an empty new database when
// existing reviews/history have not been imported. It also detects resumed
// writes in the retired source tables after cutover.
func (p *Postgres) CheckLegacyMigration(ctx context.Context, sourceURL string) error {
	source, err := pgxpool.New(ctx, sourceURL)
	if err != nil {
		return errors.New("could not check legacy creator data")
	}
	defer source.Close()
	id, err := identify(ctx, source)
	if err != nil {
		return errors.New("could not identify legacy creator database")
	}
	reviews, history, err := sourceSnapshot(ctx, source)
	if err != nil {
		return fmt.Errorf("could not check legacy creator data: %w", err)
	}
	var oldID databaseIdentity
	var oldReviews, oldHistory tableDigest
	err = p.pool.QueryRow(ctx, `SELECT source_database,source_server,review_digest,history_digest,review_count,history_count FROM creator_migration_state WHERE key='legacy_account'`).Scan(&oldID.Database, &oldID.Server, &oldReviews.Sum, &oldHistory.Sum, &oldReviews.Count, &oldHistory.Count)
	if errors.Is(err, pgx.ErrNoRows) && reviews.Count == 0 && history.Count == 0 {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: legacy creator data exists; stop the old service and run npm run db:creatplat:import before starting", ErrLegacyDataMismatch)
	}
	if err != nil {
		return err
	}
	if oldID != id || oldReviews != reviews || oldHistory != history {
		return fmt.Errorf("%w: legacy creator data changed after import; stop writers and reconcile before starting", ErrLegacyDataMismatch)
	}
	return nil
}

// CheckLegacyMigrationCounts is the cheap startup gate: it compares only the
// source identity and row counts with the import marker. It catches a missing
// import and added or removed rows; CheckLegacyMigration also catches edits.
func (p *Postgres) CheckLegacyMigrationCounts(ctx context.Context, sourceURL string) error {
	source, err := pgxpool.New(ctx, sourceURL)
	if err != nil {
		return errors.New("could not check legacy creator data")
	}
	defer source.Close()
	id, err := identify(ctx, source)
	if err != nil {
		return errors.New("could not identify legacy creator database")
	}
	var reviews, history int64
	if err := source.QueryRow(ctx, `SELECT (SELECT count(*) FROM athub_reviews),(SELECT count(*) FROM novel_revisions)`).Scan(&reviews, &history); err != nil {
		return fmt.Errorf("could not check legacy creator data: %w", err)
	}
	var oldID databaseIdentity
	var oldReviews, oldHistory int64
	err = p.pool.QueryRow(ctx, `SELECT source_database,source_server,review_count,history_count FROM creator_migration_state WHERE key='legacy_account'`).Scan(&oldID.Database, &oldID.Server, &oldReviews, &oldHistory)
	if errors.Is(err, pgx.ErrNoRows) && reviews == 0 && history == 0 {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: legacy creator data exists; stop the old service and run npm run db:creatplat:import before starting", ErrLegacyDataMismatch)
	}
	if err != nil {
		return err
	}
	if oldID != id || oldReviews != reviews || oldHistory != history {
		return fmt.Errorf("%w: legacy creator data changed after import; stop writers and reconcile before starting", ErrLegacyDataMismatch)
	}
	return nil
}

type ImportResult struct {
	Reviews, History int64
	AlreadyImported  bool
}

// ImportLegacy takes table locks, streams data, compares every copied field,
// and commits the import marker atomically with the creator rows. Source rows
// are never changed or deleted. Both old and new services must be stopped.
func ImportLegacy(ctx context.Context, source, target *pgxpool.Pool) (ImportResult, error) {
	sourceID, err := identify(ctx, source)
	if err != nil {
		return ImportResult{}, err
	}
	targetID, err := identify(ctx, target)
	if err != nil {
		return ImportResult{}, err
	}
	if sourceID == targetID {
		return ImportResult{}, errors.New("creator destination must be a separate database")
	}
	src, err := source.Begin(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	defer src.Rollback(ctx)
	if _, err := src.Exec(ctx, `LOCK TABLE athub_reviews,novel_revisions IN SHARE MODE; SET LOCAL TIME ZONE 'UTC'`); err != nil {
		return ImportResult{}, err
	}
	reviews, err := digest(ctx, src, "athub_reviews")
	if err != nil {
		return ImportResult{}, err
	}
	history, err := digest(ctx, src, "novel_revisions")
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{Reviews: reviews.Count, History: history.Count}
	dst, err := target.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer dst.Rollback(ctx)
	if _, err := dst.Exec(ctx, `LOCK TABLE athub_reviews,novel_revisions,creator_migration_state IN EXCLUSIVE MODE; SET LOCAL TIME ZONE 'UTC'`); err != nil {
		return result, err
	}
	var previousID databaseIdentity
	var previousReviews, previousHistory tableDigest
	err = dst.QueryRow(ctx, `SELECT source_database,source_server,review_digest,history_digest,review_count,history_count FROM creator_migration_state WHERE key='legacy_account'`).Scan(&previousID.Database, &previousID.Server, &previousReviews.Sum, &previousHistory.Sum, &previousReviews.Count, &previousHistory.Count)
	if err == nil {
		if previousID != sourceID || previousReviews != reviews || previousHistory != history {
			return result, errors.New("source changed since import; automatic overwrite is forbidden")
		}
		result.AlreadyImported = true
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	var existing int64
	if err := dst.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM athub_reviews)+(SELECT COUNT(*) FROM novel_revisions)`).Scan(&existing); err != nil {
		return result, err
	}
	if existing != 0 {
		return result, errors.New("creator destination is not empty; refusing to overwrite its data")
	}
	for _, item := range []struct {
		table   string
		columns []string
	}{
		{"athub_reviews", []string{"id", "user_id", "contributor", "operation", "method", "path", "query", "payload", "status", "reviewer_id", "reviewer", "note", "result_status", "result", "created_at", "reviewed_at"}},
		{"novel_revisions", []string{"id", "batch_id", "novel_id", "user_id", "contributor", "changes", "status", "created_at", "completed_at"}},
	} {
		query := `SELECT `
		columns, parameters := "", ""
		for i, col := range item.columns {
			if i > 0 {
				query += ","
				columns += ","
				parameters += ","
			}
			query += pgx.Identifier{col}.Sanitize()
			columns += pgx.Identifier{col}.Sanitize()
			parameters += fmt.Sprintf("$%d", i+1)
			switch col {
			case "contributor", "payload", "reviewer", "result", "changes":
				// Preserve JSON numeric precision and SQL NULL without decoding
				// through map[string]any or relying on binary COPY support.
				query += "::text"
				parameters += "::jsonb"
			}
		}
		query += ` FROM ` + pgx.Identifier{item.table}.Sanitize() + ` ORDER BY id`
		rows, err := src.Query(ctx, query)
		if err != nil {
			return result, err
		}
		insert := `INSERT INTO ` + pgx.Identifier{item.table}.Sanitize() + ` (` + columns + `) VALUES (` + parameters + `)`
		for rows.Next() {
			values, valueErr := rows.Values()
			if valueErr != nil {
				rows.Close()
				return result, valueErr
			}
			if _, err := dst.Exec(ctx, insert, values...); err != nil {
				rows.Close()
				return result, err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
	}
	copiedReviews, err := digest(ctx, dst, "athub_reviews")
	if err != nil {
		return result, err
	}
	copiedHistory, err := digest(ctx, dst, "novel_revisions")
	if err != nil {
		return result, err
	}
	if reviews != copiedReviews || history != copiedHistory {
		return result, errors.New("creator import verification failed; destination transaction rolled back")
	}
	if _, err := dst.Exec(ctx, `INSERT INTO creator_migration_state(key,source_database,source_server,review_digest,history_digest,review_count,history_count) VALUES('legacy_account',$1,$2,$3,$4,$5,$6)`, sourceID.Database, sourceID.Server, reviews.Sum, history.Sum, reviews.Count, history.Count); err != nil {
		return result, err
	}
	if err := dst.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}
