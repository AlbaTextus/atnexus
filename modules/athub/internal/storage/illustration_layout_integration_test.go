package storage

import (
	"context"
	"strings"
	"testing"
)

func testIllustrationLayout(t *testing.T, ctx context.Context, st *Postgres) {
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "layout test"})
	if err != nil {
		t.Fatal(err)
	}
	book, err := st.CreateNovel(ctx, NovelInput{PlatformID: platform.ID, Title: "Layout", Status: "ONGOING"})
	if err != nil {
		t.Fatal(err)
	}
	image, err := st.AddIllustration(ctx, book.ID, IllustrationInput{URL: "https://images.example.test/layout.jpg", SHA256: strings.Repeat("c", 64), Width: 300, Height: 200})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := st.GetIllustrationLayout(ctx, book.ID)
	if err != nil || initial.Revision != 0 || len(initial.Blocks) != 0 {
		t.Fatalf("unexpected initial layout: %+v %v", initial, err)
	}
	input := IllustrationLayout{Blocks: []IllustrationBlock{{ImageID: image.ID, X: 15, Y: 30, Width: 400, Height: 250}}}
	var saved IllustrationLayout
	err = st.Mutate(ctx, func(ctx context.Context) error {
		saved, err = st.SaveIllustrationLayout(ctx, book.ID, input)
		return err
	})
	if err != nil || saved.Revision != 1 {
		t.Fatalf("save: %+v %v", saved, err)
	}
	read, err := st.GetIllustrationLayout(ctx, book.ID)
	if err != nil || read.Blocks[0] != input.Blocks[0] {
		t.Fatalf("layout not persisted: %+v %v", read, err)
	}
	if _, err := st.SaveIllustrationLayout(ctx, book.ID, input); err != ErrConflict {
		t.Fatalf("stale layout overwrote newer revision: %v", err)
	}
	input.Revision = saved.Revision
	input.Blocks[0].ImageID = image.ID + 1
	if _, err := st.SaveIllustrationLayout(ctx, book.ID, input); err != ErrInvalidReference {
		t.Fatalf("invalid resource accepted: %v", err)
	}
	var history []RevisionDraft
	st.RecordChanges = func(_ context.Context, _ string, value []RevisionDraft) (func(context.Context, bool) error, error) {
		history = append(history, value...)
		return func(context.Context, bool) error { return nil }, nil
	}
	defer func() { st.RecordChanges = nil }()
	if err := st.Mutate(ctx, func(ctx context.Context) error { return st.DeleteIllustration(ctx, book.ID, image.ID) }); err != nil {
		t.Fatal(err)
	}
	read, err = st.GetIllustrationLayout(ctx, book.ID)
	if err != nil || len(read.Blocks) != 0 || read.Revision != 2 {
		t.Fatalf("deleted image left a stale block: %+v %v", read, err)
	}
	if len(history) != 1 || len(history[0].Changes) != 2 {
		t.Fatalf("delete and layout were not audited together: %+v", history)
	}
}
