package storage

import (
	"context"
	"strings"
	"testing"
)

func testIllustrationDeletion(t *testing.T, ctx context.Context, st *Postgres) {
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "illustration deletion"})
	if err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNovel(ctx, NovelInput{PlatformID: platform.ID, Title: "Illustrations", Status: "ONGOING"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateNovel(ctx, NovelInput{PlatformID: platform.ID, Title: "Other novel", Status: "ONGOING"})
	if err != nil {
		t.Fatal(err)
	}
	image, err := st.AddIllustration(ctx, book.ID, IllustrationInput{
		URL: "https://images.example.test/image.jpg", SHA256: strings.Repeat("a", 64), Width: 320, Height: 240,
	})
	if err != nil {
		t.Fatal(err)
	}
	var changes []RevisionDraft
	st.RecordChanges = func(_ context.Context, _ string, drafts []RevisionDraft) (func(context.Context, bool) error, error) {
		changes = append(changes, drafts...)
		return func(_ context.Context, committed bool) error {
			if !committed {
				t.Error("illustration deletion did not commit")
			}
			return nil
		}, nil
	}
	defer func() { st.RecordChanges = nil }()
	if err := st.Mutate(ctx, func(ctx context.Context) error { return st.DeleteIllustration(ctx, other.ID, image.ID) }); err != ErrNotFound {
		t.Fatalf("wrong parent accepted: %v", err)
	}
	if len(changes) != 0 {
		t.Fatal("failed delete created history")
	}
	items, err := st.ListIllustrations(ctx, book.ID, 100, 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("wrong-parent deletion changed images: %+v %v", items, err)
	}
	ctx = WithContributor(ctx, Contributor{ID: "test-editor", Name: "编辑验证"})
	if err := st.Mutate(ctx, func(ctx context.Context) error { return st.DeleteIllustration(ctx, book.ID, image.ID) }); err != nil {
		t.Fatal(err)
	}
	items, err = st.ListIllustrations(ctx, book.ID, 100, 0)
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted image remains: %+v %v", items, err)
	}
	if len(changes) != 1 || changes[0].NovelID != book.ID || changes[0].Contributor.ID != "test-editor" ||
		len(changes[0].Changes) != 1 || changes[0].Changes[0].Entity != "Illustration" || changes[0].Changes[0].Action != "DELETE" {
		t.Fatalf("delete history not attributed correctly: %+v", changes)
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error { return st.DeleteIllustration(ctx, book.ID, image.ID) }); err != ErrNotFound {
		t.Fatalf("repeated delete: %v", err)
	}
}
