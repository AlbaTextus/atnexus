package storage

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
)

// Resolve names inside the novel transaction. Shared catalog rows are never
// renamed: only this novel's author and tag references are replaced.
func resolveNovelReferences(ctx context.Context, tx pgx.Tx, n *NovelInput) error {
	if n.AuthorName != nil && n.AuthorID == nil {
		id, err := uploadAuthor(ctx, tx, n.PlatformID, *n.AuthorName)
		if err != nil {
			return err
		}
		n.AuthorID = &id
	}
	// A consistent order prevents concurrent edits from locking tags in reverse.
	names := append([]string(nil), n.TagNames...)
	sort.Strings(names)
	ids := append([]int32(nil), n.TagIDs...)
	for _, name := range names {
		var id int32
		err := tx.QueryRow(ctx, `INSERT INTO "Tag" ("name") VALUES ($1)
			ON CONFLICT ("name") DO NOTHING RETURNING "id"`, name).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT "id" FROM "Tag" WHERE "name"=$1 FOR KEY SHARE`, name).Scan(&id)
		}
		if err != nil {
			return databaseError(err)
		}
		ids = append(ids, id)
	}
	n.TagIDs = ids
	return nil
}
