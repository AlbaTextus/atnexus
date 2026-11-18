package storage

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func testNovelReferenceNames(t *testing.T, ctx context.Context, st *Postgres) {
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "local reference edits"})
	if err != nil {
		t.Fatal(err)
	}
	originalName := "共享作者"
	author, err := st.CreateAuthor(ctx, AuthorInput{PlatformID: platform.ID, Name: &originalName})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := st.CreateTag(ctx, TagInput{Name: "共享标签"})
	if err != nil {
		t.Fatal(err)
	}
	input := NovelInput{PlatformID: platform.ID, AuthorID: &author.ID, Title: "first", Status: "ONGOING", TagIDs: []int32{tag.ID}}
	first, err := st.CreateNovel(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Title = "second"
	second, err := st.CreateNovel(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	newName := "仅当前作品的作者"
	input.AuthorID, input.AuthorName, input.TagIDs, input.TagNames = nil, &newName, nil, []string{"仅当前作品的标签"}
	edited, err := st.UpdateNovel(ctx, first.ID, input)
	if err != nil || edited.AuthorID == nil || *edited.AuthorID == author.ID || len(edited.TagIDs) != 1 || edited.TagIDs[0] == tag.ID {
		t.Fatalf("local edit: %+v %v", edited, err)
	}
	if got, err := st.GetNovel(ctx, second.ID); err != nil || !reflect.DeepEqual(got, second) {
		t.Fatalf("changed another novel: %+v %v", got, err)
	}
	if got, err := st.GetAuthor(ctx, author.ID); err != nil || got.Name == nil || *got.Name != originalName {
		t.Fatalf("renamed shared author: %+v %v", got, err)
	}
	if got, err := st.GetTag(ctx, tag.ID); err != nil || got.Name != tag.Name {
		t.Fatalf("renamed shared tag: %+v %v", got, err)
	}
	if got, err := st.GetNovel(ctx, first.ID); err != nil || !reflect.DeepEqual(got, edited) {
		t.Fatalf("edit did not persist: %+v %v", got, err)
	}
	reused, err := st.UpdateNovel(ctx, first.ID, input)
	if err != nil || reused.AuthorID == nil || *reused.AuthorID != *edited.AuthorID || !reflect.DeepEqual(reused.TagIDs, edited.TagIDs) {
		t.Fatalf("did not reuse names: %+v %v", reused, err)
	}
	// A failure after name resolution must roll back newly created catalog rows.
	failedName, failedTag := "回滚作者", "回滚标签"
	input.AuthorName, input.TagIDs, input.TagNames = &failedName, []int32{2147483647}, []string{failedTag}
	if _, err := st.UpdateNovel(ctx, first.ID, input); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("expected failed association: %v", err)
	}
	var leftovers int
	if err := st.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM "Author" WHERE "platformId"=$1 AND "name"=$2) +
		(SELECT count(*) FROM "Tag" WHERE "name"=$3)`, platform.ID, failedName, failedTag).Scan(&leftovers); err != nil || leftovers != 0 {
		t.Fatalf("failed edit left catalog rows: %d %v", leftovers, err)
	}
	if got, err := st.GetNovel(ctx, first.ID); err != nil || !reflect.DeepEqual(got, reused) {
		t.Fatalf("failed edit changed novel: %+v %v", got, err)
	}
	input.AuthorName, input.TagIDs, input.TagNames = nil, nil, nil
	if got, err := st.UpdateNovel(ctx, first.ID, input); err != nil || got.AuthorID != nil || len(got.TagIDs) != 0 {
		t.Fatalf("clear references: %+v %v", got, err)
	}
}
