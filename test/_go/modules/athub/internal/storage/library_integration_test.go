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
	t.Run("author name index", func(t *testing.T) { testAuthorNameIndex(t, ctx, st) })
}

func testAuthorNameIndex(t *testing.T, ctx context.Context, st *Postgres) {
	first, err := st.CreatePlatform(ctx, PlatformInput{Name: "author index first source"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreatePlatform(ctx, PlatformInput{Name: "author index second source"})
	if err != nil {
		t.Fatal(err)
	}
	name, padded, different, blank := "索引作者", " 索引作者 ", "另一个索引作者", "   "
	var authors []Author
	for _, input := range []AuthorInput{
		{PlatformID: first.ID, Name: &name},
		{PlatformID: first.ID, Name: &padded},
		{PlatformID: second.ID, Name: &name},
		{PlatformID: second.ID, Name: &different},
		{PlatformID: first.ID},
		{PlatformID: second.ID},
		{PlatformID: first.ID, Name: &blank},
		{PlatformID: second.ID, Name: &blank},
	} {
		author, err := st.CreateAuthor(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		authors = append(authors, author)
		if _, err := st.CreateNovel(ctx, NovelInput{PlatformID: input.PlatformID, AuthorID: &author.ID, Title: "author index book", Status: "ONGOING"}); err != nil {
			t.Fatal(err)
		}
	}
	for i, author := range authors {
		want := 1
		if i < 3 {
			want = 3
		}
		filter := NovelFilter{AuthorID: author.ID, Limit: 20}
		books, err := st.ListNovels(ctx, filter)
		if err != nil || len(books) != want {
			t.Fatalf("author %d list: count=%d want=%d err=%v", i, len(books), want, err)
		}
		page, err := st.ListNovelPage(ctx, NovelPageFilter{NovelFilter: filter})
		if err != nil || page.Total != int64(want) || len(page.Items) != want {
			t.Fatalf("author %d page: %+v want=%d err=%v", i, page, want, err)
		}
	}
	filter := NovelFilter{AuthorID: authors[2].ID, PlatformID: first.ID, Limit: 1, Offset: 1}
	books, err := st.ListNovels(ctx, filter)
	if err != nil || len(books) != 1 || books[0].PlatformID != first.ID {
		t.Fatalf("platform-filtered author list: %+v err=%v", books, err)
	}
	page, err := st.ListNovelPage(ctx, NovelPageFilter{NovelFilter: filter})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].PlatformID != first.ID {
		t.Fatalf("platform-filtered author page: %+v err=%v", page, err)
	}
	filter.AuthorID, filter.Offset = 2147483647, 0
	books, err = st.ListNovels(ctx, filter)
	if err != nil || len(books) != 0 {
		t.Fatalf("missing author list: %+v err=%v", books, err)
	}
	page, err = st.ListNovelPage(ctx, NovelPageFilter{NovelFilter: filter})
	if err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("missing author page: %+v err=%v", page, err)
	}
}
