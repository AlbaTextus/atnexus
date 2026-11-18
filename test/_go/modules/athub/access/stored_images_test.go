package athubaccess

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/athub"
	"github.com/google/uuid"
)

const hostedFiles = "https://images.test/file/"

// storedImages stands in for the image host and for the content database's
// view of the images it points at. Events are noted in the order they happen.
type storedImages struct {
	missing    map[string]bool
	used       map[string]bool
	missingErr error
	holdErr    error
	useErr     error
	events     []string
}

func (s *storedImages) Remove(_ context.Context, category, entityID, location string) error {
	s.events = append(s.events, "remove "+category+" "+entityID+" "+location)
	return nil
}

// Another spelling of a hosted address carries a query.
func (s *storedImages) Address(location string) string {
	if !strings.HasPrefix(location, hostedFiles) {
		return ""
	}
	address, _, _ := strings.Cut(location, "?")
	return address
}

func (s *storedImages) Missing(_ context.Context, location string) (bool, error) {
	s.events = append(s.events, "check "+location)
	return s.missing[location], s.missingErr
}

func (s *storedImages) HoldImage(_ context.Context, location string) error {
	s.events = append(s.events, "hold "+location)
	return s.holdErr
}

func (s *storedImages) ReleaseImage(ctx context.Context, location string, remove func(context.Context) error) error {
	if s.useErr != nil || s.used[location] {
		return s.useErr
	}
	return remove(ctx)
}

func (s *storedImages) removed() []string {
	return slices.DeleteFunc(slices.Clone(s.events), func(event string) bool { return !strings.HasPrefix(event, "remove ") })
}

type creatorRecords struct {
	store.NovelHistoryStore
	store.ATHubReviewStore
	pending    map[string]bool
	pendingErr error
	prepared   int
	finished   []bool
}

func (c *creatorRecords) PrepareNovelRevisions(context.Context, uuid.UUID, []store.NovelRevision) error {
	c.prepared++
	return nil
}
func (c *creatorRecords) FinishNovelRevisions(_ context.Context, _ uuid.UUID, committed bool) error {
	c.finished = append(c.finished, committed)
	return nil
}
func (c *creatorRecords) PendingReviewMentions(_ context.Context, text string) (bool, error) {
	return c.pending[text], c.pendingErr
}

func imageAccess(t *testing.T, images *storedImages, creator *creatorRecords) *Access {
	t.Helper()
	access, err := New(&accessStore{}, ":8003", nil)
	if err != nil {
		t.Fatal(err)
	}
	access.EnableCreatorStorage(creator)
	access.EnableImageCleanup(images, images)
	return access
}

func novelChange(action string, before, after map[string]any) athub.RevisionDraft {
	return athub.RevisionDraft{NovelID: 2, Contributor: athub.Contributor{ID: uuid.NewString(), Name: "编辑"},
		Changes: []athub.Change{{Entity: "Novel", Action: action, Before: before, After: after}}}
}

func coverChange(before, after any) athub.RevisionDraft {
	return novelChange("UPDATE", map[string]any{"coverUrl": before}, map[string]any{"coverUrl": after})
}

func TestReplacedCoverIsRemovedOnceNothingRefersToIt(t *testing.T) {
	const old, kept, awaited, gone = hostedFiles + "book/u/covers/old.jpg", hostedFiles + "book/u/covers/kept.jpg", hostedFiles + "book/u/covers/awaited.jpg", hostedFiles + "book/u/covers/gone.jpg"
	drafts := []athub.RevisionDraft{
		coverChange(old, hostedFiles+"book/u/covers/new.jpg"),
		coverChange(kept, nil),    // another novel still shows it
		coverChange(awaited, nil), // a pending review would restore it
		coverChange(nil, hostedFiles+"book/u/covers/first.jpg"),
		coverChange(hostedFiles+"book/u/covers/same.jpg", hostedFiles+"book/u/covers/same.jpg"),
		novelChange("DELETE", map[string]any{"coverUrl": gone}, nil),
	}
	run := func(committed bool, images *storedImages, creator *creatorRecords) []string {
		t.Helper()
		finish, err := imageAccess(t, images, creator).RecordChanges(context.Background(), uuid.NewString(), drafts)
		if err != nil {
			t.Fatal(err)
		}
		if removed := images.removed(); len(removed) != 0 {
			t.Fatalf("removed %v before the change was saved", removed)
		}
		if err := finish(context.Background(), committed); err != nil || len(creator.finished) != 1 || creator.finished[0] != committed {
			t.Fatalf("finish err=%v finished=%v", err, creator.finished)
		}
		return images.removed()
	}
	shown := map[string]bool{kept: true}

	removed := run(true, &storedImages{used: shown}, &creatorRecords{pending: map[string]bool{awaited: true}})
	if !slices.Equal(removed, []string{"remove book/covers  " + old, "remove book/covers  " + gone}) {
		t.Fatalf("removed=%v", removed)
	}
	if removed = run(false, &storedImages{used: shown}, &creatorRecords{}); len(removed) != 0 {
		t.Fatalf("a change that was not saved removed %v", removed)
	}
	if removed = run(true, &storedImages{useErr: errors.New("content database down")}, &creatorRecords{}); len(removed) != 0 {
		t.Fatalf("removed %v without knowing their use", removed)
	}
	if removed = run(true, &storedImages{used: shown}, &creatorRecords{pendingErr: errors.New("creator database down")}); len(removed) != 0 {
		t.Fatalf("removed %v without knowing the pending reviews", removed)
	}
}

// A decided illustration review hands its image over the same way.
func TestReleasedImageIsKeptWhileItIsReferred(t *testing.T) {
	const image = hostedFiles + "book/17/illustrations/1.jpg"
	for name, scenario := range map[string]struct {
		images  storedImages
		creator creatorRecords
		removed bool
	}{
		"unused":          {removed: true},
		"an illustration": {images: storedImages{used: map[string]bool{image: true}}},
		"awaiting review": {creator: creatorRecords{pending: map[string]bool{image: true}}},
		"use unknown":     {images: storedImages{useErr: context.DeadlineExceeded}},
		"reviews unknown": {creator: creatorRecords{pendingErr: context.DeadlineExceeded}},
	} {
		t.Run(name, func(t *testing.T) {
			images, creator := scenario.images, scenario.creator
			imageAccess(t, &images, &creator).ReleaseImage(context.Background(), "book/illustrations", "17", image)
			removed := images.removed()
			if scenario.removed != (len(removed) == 1 && removed[0] == "remove book/illustrations 17 "+image) || (len(removed) > 0) != scenario.removed {
				t.Fatalf("removed=%v", removed)
			}
		})
	}
	// Nothing to release, and a host that keeps no reviews, leave files alone.
	images := &storedImages{}
	imageAccess(t, images, &creatorRecords{}).ReleaseImage(context.Background(), "book/illustrations", "17", "")
	access, _ := New(&accessStore{}, ":8003", nil)
	access.EnableImageCleanup(images, images)
	access.ReleaseImage(context.Background(), "book/illustrations", "17", image)
	if len(images.events) != 0 {
		t.Fatalf("events=%v", images.events)
	}
}

// A cover that was replaced is deleted, and an edit form opened before that
// still carries its address. Saving it must not bring the address back.
func TestChangeCannotStartPointingAtARemovedImage(t *testing.T) {
	const removed, stored, other = hostedFiles + "book/u/covers/removed.jpg", hostedFiles + "book/u/covers/stored.jpg", hostedFiles + "book/u/covers/a-first.jpg"
	illustration := func(action string, before, after map[string]any) athub.RevisionDraft {
		draft := novelChange(action, before, after)
		draft.Changes[0].Entity = "Illustration"
		return draft
	}
	chapter := func(after map[string]any) athub.RevisionDraft {
		draft := novelChange("INSERT", nil, after)
		draft.Changes[0].Entity = "Chapter"
		return draft
	}
	for name, scenario := range map[string]struct {
		drafts  []athub.RevisionDraft
		refused bool
		events  []string
	}{
		"stale cover": {[]athub.RevisionDraft{coverChange(stored, removed)}, true, []string{"hold " + removed, "check " + removed}},
		"new novel with a removed cover": {[]athub.RevisionDraft{novelChange("INSERT", nil, map[string]any{"coverUrl": removed})}, true,
			[]string{"hold " + removed, "check " + removed}},
		"removed illustration": {[]athub.RevisionDraft{illustration("INSERT", nil, map[string]any{"url": removed})}, true,
			[]string{"hold " + removed, "check " + removed}},
		// However the address is spelled, the file is held as its removal names it.
		"another spelling": {[]athub.RevisionDraft{coverChange(nil, removed+"?v=2")}, true, []string{"hold " + removed, "check " + removed}},
		// Each image is held before it is checked, in one order for every change.
		"stored images": {[]athub.RevisionDraft{coverChange(nil, stored), illustration("INSERT", nil, map[string]any{"url": other}), coverChange(removed, stored)}, false,
			[]string{"hold " + other, "check " + other, "hold " + stored, "check " + stored}},
		// What a novel already pointed at is not this change's doing.
		"unchanged cover":     {[]athub.RevisionDraft{novelChange("UPDATE", map[string]any{"coverUrl": removed, "title": "旧"}, map[string]any{"coverUrl": removed, "title": "新"})}, false, nil},
		"cleared cover":       {[]athub.RevisionDraft{coverChange(removed, nil)}, false, nil},
		"deleted novel":       {[]athub.RevisionDraft{novelChange("DELETE", map[string]any{"coverUrl": removed}, nil)}, false, nil},
		"image hosted abroad": {[]athub.RevisionDraft{coverChange(nil, "https://elsewhere.test/file/book/u/covers/removed.jpg")}, false, nil},
		"other records":       {[]athub.RevisionDraft{chapter(map[string]any{"url": removed, "coverUrl": removed})}, false, nil},
	} {
		t.Run(name, func(t *testing.T) {
			images, creator := &storedImages{missing: map[string]bool{removed: true}}, &creatorRecords{}
			finish, err := imageAccess(t, images, creator).RecordChanges(context.Background(), uuid.NewString(), scenario.drafts)
			if !slices.Equal(images.events, scenario.events) {
				t.Fatalf("events=%v", images.events)
			}
			if scenario.refused {
				// Refused before any history was prepared for it.
				if !errors.Is(err, athub.ErrImageMissing) || finish != nil || creator.prepared != 0 {
					t.Fatalf("err=%v prepared=%d", err, creator.prepared)
				}
				return
			}
			if err != nil || finish == nil || creator.prepared != 1 {
				t.Fatalf("err=%v prepared=%d", err, creator.prepared)
			}
		})
	}
	// When the image host or the content lock cannot answer, the change is not
	// saved either: it might point at nothing.
	for name, images := range map[string]*storedImages{
		"host unavailable": {missingErr: errors.New("image database down")},
		"lock unavailable": {holdErr: context.DeadlineExceeded},
	} {
		creator := &creatorRecords{}
		finish, err := imageAccess(t, images, creator).RecordChanges(context.Background(), uuid.NewString(), []athub.RevisionDraft{coverChange(nil, stored)})
		if err == nil || errors.Is(err, athub.ErrImageMissing) || finish != nil || creator.prepared != 0 {
			t.Fatalf("%s: err=%v prepared=%d", name, err, creator.prepared)
		}
	}
	// Without an image host the adapter records changes as before.
	access, _ := New(&accessStore{}, ":8003", nil)
	creator := &creatorRecords{}
	access.EnableCreatorStorage(creator)
	if _, err := access.RecordChanges(context.Background(), uuid.NewString(), []athub.RevisionDraft{coverChange(nil, removed)}); err != nil || creator.prepared != 1 {
		t.Fatalf("err=%v prepared=%d", err, creator.prepared)
	}
}
