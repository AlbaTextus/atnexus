package storage

import (
	"context"
	"testing"
)

func testUploadAuthors(t *testing.T, ctx context.Context, st *Postgres) {
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "epub authors"})
	if err != nil {
		t.Fatal(err)
	}
	name := "EPUB 作者"
	input := UploadInput{
		AuthorName: &name,
		Novel:      &NovelInput{PlatformID: platform.ID, Title: "Book", Status: "ONGOING"},
	}
	first, err := st.Upload(ctx, input)
	if err != nil || first.Novel.AuthorID == nil {
		t.Fatalf("author not attached: %+v %v", first, err)
	}
	author, err := st.GetAuthor(ctx, *first.Novel.AuthorID)
	if err != nil || author.Name == nil || *author.Name != name || author.PlatformID != platform.ID {
		t.Fatalf("wrong author: %+v %v", author, err)
	}
	second, err := st.Upload(ctx, input)
	if err != nil || second.Novel.AuthorID == nil || *second.Novel.AuthorID != author.ID {
		t.Fatalf("same-platform author not reused: %+v %v", second, err)
	}
	other, err := st.CreatePlatform(ctx, PlatformInput{Name: "other epub platform"})
	if err != nil {
		t.Fatal(err)
	}
	input.Novel.PlatformID = other.ID
	third, err := st.Upload(ctx, input)
	if err != nil || third.Novel.AuthorID == nil || *third.Novel.AuthorID == author.ID {
		t.Fatalf("cross-platform author reused: %+v %v", third, err)
	}
	failedName := "rolled back author"
	input.AuthorName = &failedName
	missingVolume := int32(999)
	input.Chapters = []UploadChapter{{Order: 1, Title: "Chapter", Content: "Body", VolumeOrder: &missingVolume}}
	if _, err := st.Upload(ctx, input); err == nil {
		t.Fatal("invalid upload succeeded")
	}
	var count int
	if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM "Author" WHERE "name"=$1`, failedName).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed upload left an author: %d %v", count, err)
	}
}
