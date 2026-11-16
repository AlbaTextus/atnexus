package storage

import (
	"context"
	"encoding/json"
)

type NovelPageFilter struct {
	NovelFilter
	Search string
	Sort   string
}

type NovelPage struct {
	Items []Novel `json:"items"`
	Total int64   `json:"total"`
}

// ListNovelPage returns just the requested page. Author search is performed in
// SQL so the browser never needs the entire novel or author catalogue.
func (p *Postgres) ListNovelPage(ctx context.Context, f NovelPageFilter) (NovelPage, error) {
	result := NovelPage{Items: make([]Novel, 0)}
	const filter = ` FROM "Novel" n LEFT JOIN "Author" a ON a.id=n."authorId"
		WHERE ($1::integer=0 OR n."platformId"=$1) AND ` + novelAuthorFilter + `
		AND ($3::integer=0 OR EXISTS(SELECT 1 FROM "NovelTag" WHERE "novelId"=n.id AND "tagId"=$3))
		AND ($4::text='' OR strpos(lower(normalize(n.title,NFKC)),lower(normalize($4,NFKC)))>0
			OR strpos(lower(normalize(COALESCE(a.name,''),NFKC)),lower(normalize($4,NFKC)))>0)`
	args := []any{f.PlatformID, f.AuthorID, f.TagID, f.Search}
	order := `n."updatedAt" DESC,n.id DESC`
	pageOrder := `page."updatedAt" DESC,page.id DESC`
	if f.Sort == "title" {
		order = `n.title ASC,n.id ASC`
		pageOrder = `page.title ASC,page.id ASC`
	}
	// Both subqueries share one statement snapshot, including an empty page's
	// total. One round trip matters when PostgreSQL is on a remote server.
	var raw []byte
	err := p.db(ctx).QueryRow(ctx, `SELECT (SELECT count(*)`+filter+`),
		COALESCE((SELECT jsonb_agg(to_jsonb(page) ORDER BY `+pageOrder+`) FROM (
			SELECT n.*,ARRAY(SELECT "tagId" FROM "NovelTag" WHERE "novelId"=n.id ORDER BY "tagId") AS "tagIds"`+
		filter+` ORDER BY `+order+` LIMIT $5 OFFSET $6) page),'[]'::jsonb)`,
		append(args, f.Limit, f.Offset)...).Scan(&result.Total, &raw)
	if err != nil {
		return result, databaseError(err)
	}
	err = json.Unmarshal(raw, &result.Items)
	return result, err
}

func (p *Postgres) AuthorsByIDs(ctx context.Context, ids []int32) ([]Author, error) {
	rows, err := p.db(ctx).Query(ctx, `SELECT id,"platformId",name FROM "Author" WHERE id=ANY($1::integer[]) ORDER BY id`, ids)
	if err != nil {
		return nil, databaseError(err)
	}
	defer rows.Close()
	result := make([]Author, 0, len(ids))
	for rows.Next() {
		author, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, author)
	}
	return result, rows.Err()
}

func (p *Postgres) CatalogCount(ctx context.Context, resource string) (int64, error) {
	// Table names come exclusively from this allowlist.
	table := map[string]string{"novelinfo": "Novel", "authors": "Author", "tags": "Tag", "platforms": "Platform"}[resource]
	if table == "" {
		return 0, ErrNotFound
	}
	var count int64
	err := p.db(ctx).QueryRow(ctx, `SELECT count(*) FROM "`+table+`"`).Scan(&count)
	return count, err
}
