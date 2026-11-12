package storage

import (
	"context"
	"testing"
)

func testLibraryIntegration(t *testing.T, ctx context.Context, st *Postgres) {
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "pagination"})
	if err != nil {
		t.Fatal(err)
	}
	name := "ＷＲＩＴＥＲ"
	author, err := st.CreateAuthor(ctx, AuthorInput{PlatformID: platform.ID, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := st.CreateTag(ctx, TagInput{Name: "pagination"})
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"B book", "A book", "C book"} {
		if _, err := st.CreateNovel(ctx, NovelInput{PlatformID: platform.ID, AuthorID: &author.ID, Title: title, Status: "ONGOING", TagIDs: []int32{tag.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	filter := NovelPageFilter{NovelFilter: NovelFilter{PlatformID: platform.ID, TagID: tag.ID, Limit: 1, Offset: 1}, Search: "writer", Sort: "title"}
	page, err := st.ListNovelPage(ctx, filter)
	if err != nil || page.Total != 3 || len(page.Items) != 1 || page.Items[0].Title != "B book" || len(page.Items[0].TagIDs) != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	filter.Search, filter.Offset = "a BOOK", 0
	page, err = st.ListNovelPage(ctx, filter)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Title != "A book" {
		t.Fatalf("title search=%+v err=%v", page, err)
	}
	filter.Offset = 100
	page, err = st.ListNovelPage(ctx, filter)
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatalf("empty page lost total=%+v err=%v", page, err)
	}
	filter.Offset, filter.Search = 0, "%"
	page, err = st.ListNovelPage(ctx, filter)
	if err != nil || page.Total != 0 {
		t.Fatalf("search must treat SQL wildcards literally=%+v err=%v", page, err)
	}
	authors, err := st.AuthorsByIDs(ctx, []int32{author.ID, author.ID, 2147483647})
	if err != nil || len(authors) != 1 || authors[0].ID != author.ID {
		t.Fatalf("batch authors=%+v err=%v", authors, err)
	}
	for _, resource := range []string{"novelinfo", "authors", "tags", "platforms"} {
		if count, err := st.CatalogCount(ctx, resource); err != nil || count < 1 {
			t.Fatalf("count %s=%d error=%v", resource, count, err)
		}
	}
}
