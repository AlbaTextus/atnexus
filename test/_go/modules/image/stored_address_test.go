package image

import (
	"context"
	"testing"
)

// Callers decide that a file is unused by comparing addresses. Every spelling
// of a hosted address resolves to the one it was stored under, and only that
// one removes the file.
func TestStoredFileAddresses(t *testing.T) {
	m := &Module{cfg: Config{PublicURL: "https://images.test/"}}
	const stored = "https://images.test/file/book/u1/covers/1763-ab12cd34_cover.jpg"
	for location, want := range map[string]string{
		stored:          stored,
		stored + "?v=2": stored,
		stored + "#top": stored,
		"https://images.test/file/book%2Fu1%2Fcovers%2F1763-ab12cd34_cover.jpg": stored,
		"https://images.test/file/book/u1/covers/%31763-ab12cd34_cover.jpg":     stored,
		"https://images.test/file/book/u1/covers/a b.jpg":                       "https://images.test/file/book/u1/covers/a%20b.jpg",
		"https://images.test/file/book/u1/covers/a%20b.jpg":                     "https://images.test/file/book/u1/covers/a%20b.jpg",
		// Not a file of this host.
		"https://elsewhere.test/file/book/u1/covers/1.jpg": "",
		"https://images.test/files/book/u1/covers/1.jpg":   "",
		"https://images.test/file/":                        "",
		"https://images.test/file/book/../secret.jpg":      "",
		"https://images.test/file/book/u1/covers/%zz.jpg":  "",
		"": "",
	} {
		if got := m.Address(location); got != want {
			t.Errorf("Address(%q)=%q, want %q", location, got, want)
		}
	}

	// The module has no database here: reaching it would panic.
	ctx := context.Background()
	for _, location := range []string{
		stored + "?v=2",
		"https://images.test/file/book%2Fu1%2Fcovers%2F1763-ab12cd34_cover.jpg",
		"https://elsewhere.test/file/book/u1/covers/1.jpg",
		"",
	} {
		if err := m.Remove(ctx, "book/covers", "", location); err != nil {
			t.Errorf("Remove(%q)=%v", location, err)
		}
	}
	// A file of another category or entity is not this caller's to remove.
	if err := m.Remove(ctx, "book/illustrations", "", stored); err != nil {
		t.Error(err)
	}
	if err := m.Remove(ctx, "book/covers", "u2", stored); err != nil {
		t.Error(err)
	}
	if err := m.Remove(ctx, "book/covers", "", "https://images.test/file/1763-ab12cd34_cover.jpg"); err != nil {
		t.Error(err)
	}
	if missing, err := m.Missing(ctx, "https://elsewhere.test/file/book/u1/covers/1.jpg"); err != nil || missing {
		t.Errorf("a file hosted elsewhere: missing=%v err=%v", missing, err)
	}
}
