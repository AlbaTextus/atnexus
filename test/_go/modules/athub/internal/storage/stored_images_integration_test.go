package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// An image is removed only while no novel or illustration points at it, and
// its removal and a change that starts to point at it exclude each other.
func testStoredImages(t *testing.T, ctx context.Context, st *Postgres) {
	const (
		cover   = "https://images.example.test/file/book/u/covers/1.jpg"
		picture = "https://images.example.test/file/book/7/illustrations/1.jpg"
		unused  = "https://images.example.test/file/book/u/covers/2.jpg"
	)
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "stored images"})
	if err != nil {
		t.Fatal(err)
	}
	// A cover may be recorded with a query after the address it was stored under.
	spelled := cover + "?v=2"
	book, err := st.CreateNovel(ctx, NovelInput{PlatformID: platform.ID, Title: "Stored images", Status: "ONGOING", CoverURL: &spelled})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddIllustration(ctx, book.ID, IllustrationInput{URL: picture, SHA256: strings.Repeat("b", 64), Width: 320, Height: 240}); err != nil {
		t.Fatal(err)
	}
	removed := func(address string) bool {
		t.Helper()
		called := false
		if err := st.ReleaseImage(ctx, address, func(context.Context) error { called = true; return nil }); err != nil {
			t.Fatal(err)
		}
		return called
	}
	if removed(cover) || removed(picture) {
		t.Fatal("an image that content points at was removed")
	}
	if !removed(unused) {
		t.Fatal("an image nothing points at was kept")
	}
	failure := errors.New("image host unavailable")
	if err := st.ReleaseImage(ctx, unused, func(context.Context) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("a failed removal was reported as %v", err)
	}

	// While a change holds the image, its removal waits for the change to end.
	held, proceed, changed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		changed <- st.Mutate(ctx, func(ctx context.Context) error {
			if err := st.HoldImage(ctx, unused); err != nil {
				return err
			}
			close(held)
			<-proceed
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-changed:
		t.Fatalf("the change did not hold the image: %v", err)
	}
	brief, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	err = st.ReleaseImage(brief, unused, func(context.Context) error {
		t.Error("an image was removed while a change held it")
		return nil
	})
	cancel()
	if err == nil {
		t.Fatal("removal did not wait for the change")
	}
	close(proceed)
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if !removed(unused) {
		t.Fatal("the image stayed held after the change ended")
	}

	// While an image is being removed, a change cannot start to point at it.
	removing, finish, released := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		released <- st.ReleaseImage(ctx, unused, func(context.Context) error {
			close(removing)
			<-finish
			return nil
		})
	}()
	select {
	case <-removing:
	case err := <-released:
		t.Fatalf("the removal did not start: %v", err)
	}
	brief, cancel = context.WithTimeout(ctx, 300*time.Millisecond)
	err = st.Mutate(brief, func(ctx context.Context) error { return st.HoldImage(ctx, unused) })
	cancel()
	if err == nil {
		t.Fatal("a change held an image that was being removed")
	}
	close(finish)
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error { return st.HoldImage(ctx, unused) }); err != nil {
		t.Fatalf("the image stayed locked after its removal: %v", err)
	}
	// A change that fails lets go of what it held.
	if err := st.Mutate(ctx, func(ctx context.Context) error {
		if err := st.HoldImage(ctx, unused); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !removed(unused) {
		t.Fatal("a failed change kept the image held")
	}
}
