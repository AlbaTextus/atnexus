package storage

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func testTagIntegration(t *testing.T, ctx context.Context, store *Postgres) {
	platform, err := store.CreatePlatform(ctx, PlatformInput{Name: "tags"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateTag(ctx, TagInput{Name: "奇幻"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateTag(ctx, TagInput{Name: "冒险"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTag(ctx, TagInput{Name: first.Name}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate tag: %v", err)
	}
	if _, err := store.UpdateTag(ctx, second.ID, TagInput{Name: first.Name}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate rename: %v", err)
	}
	if tags, err := store.ListTags(ctx, 1, 1); err != nil || len(tags) != 1 || tags[0].ID != second.ID {
		t.Fatalf("pagination: %+v %v", tags, err)
	}
	input := NovelInput{PlatformID: platform.ID, Title: "Tagged", Status: "ONGOING", TagIDs: []int32{second.ID, first.ID}}
	novel, err := store.CreateNovel(ctx, input)
	if err != nil || !reflect.DeepEqual(novel.TagIDs, []int32{first.ID, second.ID}) {
		t.Fatalf("create references: %+v %v", novel, err)
	}
	if err := store.DeleteTag(ctx, first.ID); !errors.Is(err, ErrTagInUse) {
		t.Fatalf("delete referenced tag: %v", err)
	}
	if tag, err := store.UpdateTag(ctx, first.ID, TagInput{Name: "幻想"}); err != nil || tag.ID != first.ID {
		t.Fatalf("rename: %+v %v", tag, err)
	}
	if tag, err := store.GetTag(ctx, first.ID); err != nil || tag.Name != "幻想" {
		t.Fatalf("get renamed tag: %+v %v", tag, err)
	}
	if got, err := store.GetNovel(ctx, novel.ID); err != nil || !reflect.DeepEqual(got.TagIDs, novel.TagIDs) {
		t.Fatalf("get references: %+v %v", got, err)
	}
	if got, err := store.ListNovels(ctx, NovelFilter{PlatformID: platform.ID, TagID: first.ID, Limit: 20}); err != nil || len(got) != 1 || !reflect.DeepEqual(got[0].TagIDs, novel.TagIDs) {
		t.Fatalf("filter references: %+v %v", got, err)
	}
	input.Title, input.TagIDs = "Must roll back", []int32{first.ID, 2147483647}
	if _, err := store.UpdateNovel(ctx, novel.ID, input); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("missing tag update: %v", err)
	}
	if got, err := store.GetNovel(ctx, novel.ID); err != nil || got.Title != novel.Title || !got.UpdatedAt.Equal(novel.UpdatedAt) || !reflect.DeepEqual(got.TagIDs, novel.TagIDs) {
		t.Fatalf("failed update changed novel: %+v %v", got, err)
	}
	if _, err := store.CreateNovel(ctx, input); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("missing tag create: %v", err)
	}
	if _, err := store.Upload(ctx, UploadInput{Novel: &input}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("missing tag upload: %v", err)
	}
	if got, err := store.ListNovels(ctx, NovelFilter{PlatformID: platform.ID, Limit: 20}); err != nil || len(got) != 1 {
		t.Fatalf("failed create left novel: %+v %v", got, err)
	}
	input.Title, input.TagIDs = "Uploaded", []int32{first.ID}
	upload, err := store.Upload(ctx, UploadInput{Novel: &input})
	if err != nil || !reflect.DeepEqual(upload.Novel.TagIDs, input.TagIDs) {
		t.Fatalf("upload tags: %+v %v", upload, err)
	}
	continued, err := store.Upload(ctx, UploadInput{NovelID: &upload.Novel.ID, Chapters: []UploadChapter{{Order: 1, Title: "Chapter", Content: "Body"}}})
	if err != nil || !reflect.DeepEqual(continued.Novel.TagIDs, input.TagIDs) {
		t.Fatalf("continuation lost tags: %+v %v", continued, err)
	}
	before, err := store.ListNovels(ctx, NovelFilter{PlatformID: platform.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upload(ctx, UploadInput{Novel: &input, Chapters: []UploadChapter{{Order: 1, VolumeOrder: &second.ID, Title: "Bad", Content: "Body"}}}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("upload rollback: %v", err)
	}
	after, err := store.ListNovels(ctx, NovelFilter{PlatformID: platform.ID, Limit: 20})
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("failed upload changed novels: %v", err)
	}
	input.TagIDs = nil
	if got, err := store.UpdateNovel(ctx, novel.ID, input); err != nil || got.TagIDs == nil || len(got.TagIDs) != 0 {
		t.Fatalf("clear references: %+v %v", got, err)
	}
	if err := store.DeleteTag(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteNovel(ctx, upload.Novel.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTag(ctx, first.ID); err != nil {
		t.Fatalf("cascade left references: %v", err)
	}
	if _, err := store.GetTag(ctx, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted tag: %v", err)
	}
}
