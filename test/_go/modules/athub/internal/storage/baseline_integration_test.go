package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// baselineSuite plays what a review does with a submitted change: the change is
// rehearsed, someone else changes the content directly, and the same change is
// then applied with the baseline its rehearsal left.
type baselineSuite struct {
	ctx      context.Context
	st       *Postgres
	platform int32
	recorded [][]RevisionDraft
}

func testBaselineIntegration(t *testing.T, ctx context.Context, st *Postgres) {
	// These scenarios get an allowance of their own instead of what the tests
	// before them left of the shared one.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "baseline"})
	if err != nil {
		t.Fatal(err)
	}
	s := &baselineSuite{ctx: WithContributor(ctx, Contributor{ID: "reviewer", Name: "审核"}), st: st, platform: platform.ID}
	st.RecordChanges = func(_ context.Context, _ string, drafts []RevisionDraft) (func(context.Context, bool) error, error) {
		s.recorded = append(s.recorded, drafts)
		return func(context.Context, bool) error { return nil }, nil
	}
	defer func() { st.RecordChanges = nil }()
	t.Run("novel fields", s.novelFields)
	t.Run("reflected names", s.reflectedNames)
	t.Run("chapters", s.chapters)
	t.Run("novel deletion", s.novelDeletion)
	t.Run("upload", s.upload)
	t.Run("upload creating a novel", s.uploadCreatingNovel)
	t.Run("volumes", s.volumes)
	t.Run("catalog records", s.catalogRecords)
	t.Run("illustration deletion", s.illustrationDeletion)
	t.Run("illustration layouts", s.illustrationLayouts)
	t.Run("documents", s.documents)
	t.Run("receipts", s.receipts)
}

// rehearse runs change the way a submission does and returns its baseline.
func (s *baselineSuite) rehearse(t *testing.T, change func(context.Context) error) []byte {
	t.Helper()
	recorded := len(s.recorded)
	ctx, rehearsal := WithRehearsal(s.ctx)
	if err := s.st.Mutate(ctx, change); err != nil {
		t.Fatalf("rehearsal: %v", err)
	}
	if len(s.recorded) != recorded {
		t.Fatal("rehearsal recorded a revision")
	}
	baseline := rehearsal.Baseline()
	if baseline == nil {
		t.Fatal("rehearsal left no baseline")
	}
	return baseline
}

// approve applies change with its baseline. The baseline first goes through a
// JSONB value, which is how the host keeps it until the review is decided.
func (s *baselineSuite) approve(t *testing.T, baseline []byte, change func(context.Context) error) error {
	t.Helper()
	var stored []byte
	if err := s.st.pool.QueryRow(s.ctx, `SELECT $1::jsonb`, baseline).Scan(&stored); err != nil {
		t.Fatalf("keeping the baseline: %v", err)
	}
	return s.st.Mutate(WithBaseline(s.ctx, stored), change)
}

// applied requires change to be applied and returns what was recorded for it.
func (s *baselineSuite) applied(t *testing.T, baseline []byte, change func(context.Context) error) []Change {
	t.Helper()
	recorded := len(s.recorded)
	if err := s.approve(t, baseline, change); err != nil {
		t.Fatalf("change not applied: %v", err)
	}
	var changes []Change
	for _, drafts := range s.recorded[recorded:] {
		for _, draft := range drafts {
			changes = append(changes, draft.Changes...)
		}
	}
	return changes
}

// refused requires change to be refused as stale without leaving a revision.
func (s *baselineSuite) refused(t *testing.T, baseline []byte, change func(context.Context) error) {
	t.Helper()
	recorded := len(s.recorded)
	if err := s.approve(t, baseline, change); !errors.Is(err, ErrStale) {
		t.Fatalf("outdated change not refused: %v", err)
	}
	if len(s.recorded) != recorded {
		t.Fatal("refused change recorded a revision")
	}
}

func (s *baselineSuite) novel(t *testing.T, input NovelInput) Novel {
	t.Helper()
	input.PlatformID, input.Status = s.platform, "ONGOING"
	novel, err := s.st.CreateNovel(s.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return novel
}

// save writes a chapter the way someone else does while a change is waiting.
func (s *baselineSuite) save(t *testing.T, chapter ChapterInput) Chapter {
	t.Helper()
	saved, err := s.st.SaveChapter(s.ctx, chapter)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func (s *baselineSuite) chapter(t *testing.T, novelID, order int32) Chapter {
	t.Helper()
	chapter, err := s.st.GetChapter(s.ctx, novelID, order)
	if err != nil {
		t.Fatal(err)
	}
	return chapter
}

// differing lists the fields in which a recorded change differs.
func differing(change Change) []string {
	fields := make([]string, 0)
	for name, value := range change.After {
		if !reflect.DeepEqual(change.Before[name], value) {
			fields = append(fields, name)
		}
	}
	sort.Strings(fields)
	return fields
}

func actions(changes []Change) []string {
	result := make([]string, 0, len(changes))
	for _, change := range changes {
		result = append(result, change.Entity+" "+change.Action)
	}
	sort.Strings(result)
	return result
}

func (s *baselineSuite) novelFields(t *testing.T) {
	st, ctx := s.st, s.ctx
	first, second := "https://images.example.test/first.jpg", "https://images.example.test/second.jpg"
	form := NovelInput{PlatformID: s.platform, Title: "Draft title", CoverURL: &first, Status: "ONGOING", TagIDs: []int32{}}
	novel := s.novel(t, form)
	// The form still holds the cover it was opened with.
	edit := form
	edit.Title = "Submitted title"
	change := func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, novel.ID, edit); return err }
	baseline := s.rehearse(t, change)
	if got, err := st.GetNovel(ctx, novel.ID); err != nil || got.Title != form.Title {
		t.Fatalf("rehearsal changed the novel: %+v %v", got, err)
	}
	recovered := form
	recovered.CoverURL = &second
	if _, err := st.UpdateNovel(ctx, novel.ID, recovered); err != nil {
		t.Fatal(err)
	}
	changes := s.applied(t, baseline, change)
	got, err := st.GetNovel(ctx, novel.ID)
	if err != nil || got.Title != edit.Title || got.CoverURL == nil || *got.CoverURL != second {
		t.Fatalf("title not applied or newer cover lost: %+v %v", got, err)
	}
	if len(changes) != 1 || changes[0].Entity != "Novel" || changes[0].Action != "UPDATE" || !reflect.DeepEqual(differing(changes[0]), []string{"title"}) {
		t.Fatalf("recorded more than the title: %+v", changes)
	}

	// Applied without a baseline the form overwrites the newer cover, as it
	// did before reviews kept one. A nil baseline asks for exactly that.
	edit.Title = "Submitted again"
	if err := st.Mutate(WithBaseline(ctx, nil), change); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetNovel(ctx, novel.ID); err != nil || got.Title != edit.Title || got.CoverURL == nil || *got.CoverURL != first {
		t.Fatalf("change without a baseline not applied in full: %+v %v", got, err)
	}

	// The same kind of edit is written over a title someone else has given the
	// novel since: of two changes to one field the one applied later stands. The
	// summary it did not alter keeps what it was given since.
	other := s.novel(t, form)
	change = func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, other.ID, edit); return err }
	baseline = s.rehearse(t, change)
	renamed := form
	renamed.Title, renamed.Summary = "Someone else's title", &second
	if _, err := st.UpdateNovel(ctx, other.ID, renamed); err != nil {
		t.Fatal(err)
	}
	changes = s.applied(t, baseline, change)
	if got, err := st.GetNovel(ctx, other.ID); err != nil || got.Title != edit.Title || got.Summary == nil || *got.Summary != second {
		t.Fatalf("title not written over the newer one, or newer summary lost: %+v %v", got, err)
	}
	// History shows the title that was written over.
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"title"}) || changes[0].Before["title"] != renamed.Title {
		t.Fatalf("recorded more than the title, or not the title it replaced: %+v", changes)
	}
	// Without a baseline the form also clears the summary, which it never held.
	if err := st.Mutate(ctx, change); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetNovel(ctx, other.ID); err != nil || got.Title != edit.Title || got.Summary != nil {
		t.Fatalf("change without a baseline not applied in full: %+v %v", got, err)
	}

	// Every field an edit altered is written over what the field holds by now:
	// here the summary it cleared, and the cover, the status and the tags it
	// replaced. The title it left alone keeps what it was given since.
	ours, err := st.CreateTag(ctx, TagInput{Name: "baseline tag of the submitter"})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := st.CreateTag(ctx, TagInput{Name: "baseline tag of someone else"})
	if err != nil {
		t.Fatal(err)
	}
	summary, third := "Summary", "https://images.example.test/third.jpg"
	held := NovelInput{PlatformID: s.platform, Title: "Contested", Summary: &summary, CoverURL: &first, Status: "ONGOING", TagIDs: []int32{}}
	contested := s.novel(t, held)
	revised := held
	revised.Summary, revised.CoverURL, revised.Status, revised.TagIDs = nil, &second, "COMPLETED", []int32{ours.ID}
	change = func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, contested.ID, revised); return err }
	baseline = s.rehearse(t, change)
	// The baseline tells what the edit makes of each field, for showing the
	// edit while it waits.
	queued, err := SubmittedChanges(baseline, contested.ID)
	if want := []SubmittedChange{{Entity: "Novel", Action: "UPDATE", ID: float64(contested.ID), Fields: []string{"coverUrl", "status", "summary", "tagIds"},
		After: map[string]any{"coverUrl": second, "status": "COMPLETED", "summary": nil, "tagIds": []any{float64(ours.ID)}, "tagNames": []any{ours.Name}}}}; err != nil || !reflect.DeepEqual(queued, want) {
		t.Fatalf("queued edit: %+v %v, want %+v", queued, err, want)
	}
	theirSummary := "Summary by someone else"
	if _, err := st.UpdateNovel(ctx, contested.ID, NovelInput{PlatformID: s.platform, Title: "Contested, retitled by someone else",
		Summary: &theirSummary, CoverURL: &third, Status: "ABANDONED", TagIDs: []int32{theirs.ID}}); err != nil {
		t.Fatal(err)
	}
	changes = s.applied(t, baseline, change)
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"coverUrl", "status", "summary", "tagIds", "tagNames"}) {
		t.Fatalf("recorded other fields than the edit altered: %+v", changes)
	}
	got, err = st.GetNovel(ctx, contested.ID)
	if err != nil || got.Title != "Contested, retitled by someone else" || got.Summary != nil || got.CoverURL == nil || *got.CoverURL != second ||
		got.Status != "COMPLETED" || !reflect.DeepEqual(got.TagIDs, []int32{ours.ID}) {
		t.Fatalf("altered fields not written over the newer ones, or newer title lost: %+v %v", got, err)
	}

	// A change that altered nothing when it was submitted alters nothing when
	// it is approved, whatever its form has missed since.
	unchanged := s.novel(t, form)
	change = func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, unchanged.ID, form); return err }
	baseline = s.rehearse(t, change)
	var document map[string]any
	if err := json.Unmarshal(baseline, &document); err != nil || len(document) != 1 || document["v"] != float64(1) {
		t.Fatalf("baseline of an empty change lists something: %s %v", baseline, err)
	}
	if _, err := st.UpdateNovel(ctx, unchanged.ID, renamed); err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, change); len(changes) != 0 {
		t.Fatalf("empty change altered the novel: %+v", changes)
	}
	if got, err := st.GetNovel(ctx, unchanged.ID); err != nil || got.Title != renamed.Title || got.Summary == nil {
		t.Fatalf("empty change altered the novel: %+v %v", got, err)
	}

	// The form may be outdated in everything it holds: while its new title was
	// waiting, the novel moved to another platform and author and was given a
	// summary, a cover, a status and a tag.
	elsewhere, err := st.CreatePlatform(ctx, PlatformInput{Name: "baseline, elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	penName := "Baseline author elsewhere"
	author, err := st.CreateAuthor(ctx, AuthorInput{PlatformID: elsewhere.ID, Name: &penName})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := st.CreateTag(ctx, TagInput{Name: "baseline novel fields"})
	if err != nil {
		t.Fatal(err)
	}
	moved := s.novel(t, form)
	change = func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, moved.ID, edit); return err }
	baseline = s.rehearse(t, change)
	if _, err := st.UpdateNovel(ctx, moved.ID, NovelInput{PlatformID: elsewhere.ID, AuthorID: &author.ID, Title: form.Title,
		Summary: &second, CoverURL: &second, Status: "COMPLETED", TagIDs: []int32{tag.ID}}); err != nil {
		t.Fatal(err)
	}
	changes = s.applied(t, baseline, change)
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"title"}) {
		t.Fatalf("recorded more than the title: %+v", changes)
	}
	got, err = st.GetNovel(ctx, moved.ID)
	if err != nil || got.Title != edit.Title || got.PlatformID != elsewhere.ID || got.AuthorID == nil || *got.AuthorID != author.ID ||
		got.Summary == nil || *got.Summary != second || got.CoverURL == nil || *got.CoverURL != second ||
		got.Status != "COMPLETED" || !reflect.DeepEqual(got.TagIDs, []int32{tag.ID}) {
		t.Fatalf("title not applied or newer fields lost: %+v %v", got, err)
	}
}

func (s *baselineSuite) reflectedNames(t *testing.T) {
	st, ctx := s.st, s.ctx
	penName, tagName := "Baseline pen name", "baseline reflected tag"
	author, err := st.CreateAuthor(ctx, AuthorInput{PlatformID: s.platform, Name: &penName})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := st.CreateTag(ctx, TagInput{Name: tagName})
	if err != nil {
		t.Fatal(err)
	}
	novel := s.novel(t, NovelInput{AuthorID: &author.ID, Title: "Reflected", TagIDs: []int32{tag.ID}})
	// The form names the author and the tag instead of pointing at them.
	edit := NovelInput{PlatformID: s.platform, AuthorName: &penName, TagNames: []string{tagName}, Title: "Reflected, edited", Status: "ONGOING", TagIDs: []int32{}}
	change := func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, novel.ID, edit); return err }
	baseline := s.rehearse(t, change)
	newPenName := "Baseline pen name, renamed"
	if _, err := st.UpdateAuthorName(ctx, author.ID, &newPenName); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTag(ctx, tag.ID, TagInput{Name: tagName + ", renamed"}); err != nil {
		t.Fatal(err)
	}
	changes := s.applied(t, baseline, change)
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"title"}) {
		t.Fatalf("renamed author or tag changed what was applied: %+v", changes)
	}
	got, err := st.GetNovel(ctx, novel.ID)
	if err != nil || got.Title != edit.Title || got.AuthorID == nil || *got.AuthorID != author.ID || !reflect.DeepEqual(got.TagIDs, []int32{tag.ID}) {
		t.Fatalf("novel no longer points at its author and tag: %+v %v", got, err)
	}
	// Names the edit left alone are not looked up again, so the old spellings
	// do not come back as new records.
	var strays int
	if err := st.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM "Author" WHERE name=$1)+(SELECT count(*) FROM "Tag" WHERE name=$2)`,
		penName, tagName).Scan(&strays); err != nil || strays != 0 {
		t.Fatalf("old names were created again: %d %v", strays, err)
	}
}

func (s *baselineSuite) chapters(t *testing.T) {
	st, ctx := s.st, s.ctx
	novel := s.novel(t, NovelInput{Title: "Chapters"})
	volume, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 1, Title: "Volume"})
	if err != nil {
		t.Fatal(err)
	}
	var edit ChapterInput
	change := func(ctx context.Context) error { _, err := st.SaveChapter(ctx, edit); return err }

	// A new title is applied to a chapter that was rewritten and put into a
	// volume in the meantime.
	original := ChapterInput{NovelID: novel.ID, Order: 1, Title: "Chapter one", Content: "Original body\n"}
	s.save(t, original)
	edit = original
	edit.Title = "Chapter one, retitled"
	baseline := s.rehearse(t, change)
	rewritten := original
	rewritten.VolumeID, rewritten.Content = &volume.ID, "Body rewritten by someone else\n"
	s.save(t, rewritten)
	changes := s.applied(t, baseline, change)
	if got := s.chapter(t, novel.ID, 1); got.Title != edit.Title || got.Content != rewritten.Content || got.VolumeID == nil || *got.VolumeID != volume.ID {
		t.Fatalf("title not applied, or newer body or volume lost: %+v", got)
	}
	if len(changes) != 1 || changes[0].Entity != "Chapter" || !reflect.DeepEqual(differing(changes[0]), []string{"title"}) || len(changes[0].BodyDiff) != 0 {
		t.Fatalf("recorded more than the title: %+v", changes)
	}

	// A new body is written over a body that was rewritten in the meantime: of
	// two changes to one body the one applied later stands.
	current := s.chapter(t, novel.ID, 1).ChapterInput
	edit = current
	edit.Content = "Body revised by the submitter, at some length\n"
	baseline = s.rehearse(t, change)
	rewritten = current
	rewritten.Content = "Body rewritten once more\n"
	s.save(t, rewritten)
	changes = s.applied(t, baseline, change)
	if got := s.chapter(t, novel.ID, 1); got.Content != edit.Content || got.Title != current.Title {
		t.Fatalf("body not written over the newer one: %+v", got)
	}
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"contentHash", "contentLength"}) || len(changes[0].BodyDiff) == 0 {
		t.Fatalf("body change not recorded on its own: %+v", changes)
	}

	// A body is known by its hash and its length. Rewritten into a body of the
	// same length, it alters the hash alone when it is rehearsed, and the length
	// as well once the body it is written over has another length.
	current = s.chapter(t, novel.ID, 1).ChapterInput
	edit = current
	edit.Content = strings.Replace(current.Content, "revised", "amended", 1)
	if edit.Content == current.Content || len(edit.Content) != len(current.Content) {
		t.Fatalf("body %q is no rewrite of the same length", edit.Content)
	}
	baseline = s.rehearse(t, change)
	if queued, err := SubmittedChanges(baseline, novel.ID); err != nil || len(queued) != 1 || !reflect.DeepEqual(queued[0].Fields, []string{"contentHash"}) {
		t.Fatalf("rewrite of the same length: %+v %v", queued, err)
	}
	rewritten = current
	rewritten.Content = "Shortened by someone else\n"
	s.save(t, rewritten)
	changes = s.applied(t, baseline, change)
	if got := s.chapter(t, novel.ID, 1); got.Content != edit.Content {
		t.Fatalf("body not written over a newer one of another length: %+v", got)
	}
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"contentHash", "contentLength"}) {
		t.Fatalf("body change not recorded on its own: %+v", changes)
	}
	// The other way round, a longer body alters the hash alone once the body
	// it is written over is that long.
	current = s.chapter(t, novel.ID, 1).ChapterInput
	edit = current
	edit.Content = current.Content + "And one more line\n"
	baseline = s.rehearse(t, change)
	rewritten = current
	rewritten.Content = strings.Repeat("x", len(edit.Content)-1) + "\n"
	s.save(t, rewritten)
	changes = s.applied(t, baseline, change)
	if got := s.chapter(t, novel.ID, 1); got.Content != edit.Content {
		t.Fatalf("body not written over a newer one of the same length: %+v", got)
	}
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"contentHash"}) {
		t.Fatalf("body change not recorded on its own: %+v", changes)
	}

	// A new body leaves alone a title the chapter was given in the meantime.
	current = s.chapter(t, novel.ID, 1).ChapterInput
	edit = current
	edit.Content = "Body revised by the submitter once more\n"
	baseline = s.rehearse(t, change)
	retitled := current
	retitled.Title = "Retitled by someone else"
	s.save(t, retitled)
	changes = s.applied(t, baseline, change)
	if got := s.chapter(t, novel.ID, 1); got.Title != retitled.Title || got.Content != edit.Content {
		t.Fatalf("body not applied or newer title lost: %+v", got)
	}
	if len(changes) != 1 || len(changes[0].BodyDiff) == 0 || strings.Contains(strings.Join(differing(changes[0]), " "), "title") {
		t.Fatalf("body change not recorded on its own: %+v", changes)
	}

	// A new title is written over a title the chapter was given in the
	// meantime, and a chapter taken out of its volume leaves the volume it was
	// moved to since. The body keeps what it holds.
	current = s.chapter(t, novel.ID, 1).ChapterInput
	edit = current
	edit.Title, edit.VolumeID = "Retitled by the submitter", nil
	baseline = s.rehearse(t, change)
	elsewhere, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 2, Title: "Another volume"})
	if err != nil {
		t.Fatal(err)
	}
	moved := current
	moved.Title, moved.VolumeID, moved.Content = "Retitled by someone else, again", &elsewhere.ID, "Body rewritten after the retitling\n"
	s.save(t, moved)
	changes = s.applied(t, baseline, change)
	if got := s.chapter(t, novel.ID, 1); got.Title != edit.Title || got.VolumeID != nil || got.Content != moved.Content {
		t.Fatalf("title or volume not written over the newer ones, or newer body lost: %+v", got)
	}
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"title", "volumeId"}) || changes[0].Before["title"] != moved.Title || len(changes[0].BodyDiff) != 0 {
		t.Fatalf("recorded more than the title and the volume, or not the title it replaced: %+v", changes)
	}

	// A chapter that was removed and written anew is another chapter, even
	// when it reads the same as the one the change was submitted for.
	edit = ChapterInput{NovelID: novel.ID, Order: 2, Title: "Chapter two", Content: "Second body\n"}
	first := s.save(t, edit)
	edit.Title = "Chapter two, retitled"
	baseline = s.rehearse(t, change)
	if err := st.DeleteChapter(ctx, novel.ID, 2); err != nil {
		t.Fatal(err)
	}
	second := s.save(t, first.ChapterInput)
	s.refused(t, baseline, change)
	if got := s.chapter(t, novel.ID, 2); second.ID == first.ID || got.ID != second.ID || got.Title != first.Title {
		t.Fatalf("refused change altered the new chapter: %+v", got)
	}

	// So is a chapter that was removed and not written anew.
	edit = s.chapter(t, novel.ID, 2).ChapterInput
	edit.Title = "Chapter two, retitled again"
	baseline = s.rehearse(t, change)
	if err := st.DeleteChapter(ctx, novel.ID, 2); err != nil {
		t.Fatal(err)
	}
	s.refused(t, baseline, change)
	if _, err := st.GetChapter(ctx, novel.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("refused change brought a removed chapter back: %v", err)
	}

	// A new chapter is refused when someone else has taken its place.
	edit = ChapterInput{NovelID: novel.ID, Order: 3, Title: "Chapter three", Content: "Third body\n"}
	baseline = s.rehearse(t, change)
	if _, err := st.GetChapter(ctx, novel.ID, 3); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rehearsal kept the chapter: %v", err)
	}
	s.save(t, ChapterInput{NovelID: novel.ID, Order: 3, Title: "Someone else's chapter three", Content: "Their body\n"})
	s.refused(t, baseline, change)
	if got := s.chapter(t, novel.ID, 3); got.Title != "Someone else's chapter three" || got.Content != "Their body\n" {
		t.Fatalf("refused change overwrote the other chapter: %+v", got)
	}

	// It is created while its place is free, and is nothing to do once the
	// same chapter is there.
	edit = ChapterInput{NovelID: novel.ID, Order: 4, Title: "Chapter four", Content: "Fourth body\n"}
	baseline = s.rehearse(t, change)
	if changes := s.applied(t, baseline, change); !reflect.DeepEqual(actions(changes), []string{"Chapter INSERT"}) {
		t.Fatalf("new chapter not recorded: %+v", changes)
	}
	if changes := s.applied(t, baseline, change); len(changes) != 0 {
		t.Fatalf("chapter that is already there was changed: %+v", changes)
	}

	// Removing a chapter: refused when the chapter was edited in the
	// meantime, done when it was not.
	remove := func(ctx context.Context) error { return st.DeleteChapter(ctx, novel.ID, 4) }
	baseline = s.rehearse(t, remove)
	if got := s.chapter(t, novel.ID, 4); got.Title != edit.Title {
		t.Fatalf("rehearsal removed the chapter: %+v", got)
	}
	edit.Content = "Fourth body, extended\n"
	s.save(t, edit)
	s.refused(t, baseline, remove)
	if got := s.chapter(t, novel.ID, 4); got.Content != edit.Content {
		t.Fatalf("refused removal altered the chapter: %+v", got)
	}
	baseline = s.rehearse(t, remove)
	if changes := s.applied(t, baseline, remove); !reflect.DeepEqual(actions(changes), []string{"Chapter DELETE"}) {
		t.Fatalf("removal not recorded: %+v", changes)
	}
	if _, err := st.GetChapter(ctx, novel.ID, 4); !errors.Is(err, ErrNotFound) {
		t.Fatalf("chapter not removed: %v", err)
	}
}

func (s *baselineSuite) novelDeletion(t *testing.T) {
	st, ctx := s.st, s.ctx
	book := func() Novel {
		novel := s.novel(t, NovelInput{Title: "Deletion"})
		if _, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 1, Title: "Volume"}); err != nil {
			t.Fatal(err)
		}
		for order := int32(1); order <= 2; order++ {
			s.save(t, ChapterInput{NovelID: novel.ID, Order: order, Title: "Chapter", Content: "Body\n"})
		}
		return novel
	}
	remove := func(id int32) func(context.Context) error {
		return func(ctx context.Context) error { return st.DeleteNovel(ctx, id) }
	}
	remains := func(id int32, chapters int) {
		t.Helper()
		if got, err := st.ListChapters(ctx, id, 100, 0); err != nil || len(got) != chapters {
			t.Fatalf("refused deletion altered the novel: %+v %v", got, err)
		}
	}

	grown := book()
	baseline := s.rehearse(t, remove(grown.ID))
	remains(grown.ID, 2)
	s.save(t, ChapterInput{NovelID: grown.ID, Order: 3, Title: "Added by someone else", Content: "Body\n"})
	s.refused(t, baseline, remove(grown.ID))
	remains(grown.ID, 3)

	edited := book()
	baseline = s.rehearse(t, remove(edited.ID))
	s.save(t, ChapterInput{NovelID: edited.ID, Order: 1, Title: "Chapter", Content: "Body, corrected by someone else\n"})
	s.refused(t, baseline, remove(edited.ID))
	remains(edited.ID, 2)

	// What is already gone no longer needs removing.
	shrunk := book()
	baseline = s.rehearse(t, remove(shrunk.ID))
	if err := st.DeleteChapter(ctx, shrunk.ID, 2); err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, remove(shrunk.ID)); !reflect.DeepEqual(actions(changes), []string{"Chapter DELETE", "Novel DELETE", "Volume DELETE"}) {
		t.Fatalf("deletion not recorded: %+v", changes)
	}
	if _, err := st.GetNovel(ctx, shrunk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("novel not deleted: %v", err)
	}
}

func (s *baselineSuite) upload(t *testing.T) {
	st, ctx := s.st, s.ctx
	novel := s.novel(t, NovelInput{Title: "Upload"})
	one := int32(1)
	stored := UploadInput{NovelID: &novel.ID, Volumes: []UploadVolume{{Order: 1, Title: "Volume one"}, {Order: 2, Title: "Volume two"}},
		Chapters: []UploadChapter{
			{Order: 1, VolumeOrder: &one, Title: "Chapter one", Content: "Body one\n"},
			{Order: 2, VolumeOrder: &one, Title: "Chapter two", Content: "Body two\n"},
		}}
	seeded, err := st.Upload(ctx, stored)
	if err != nil {
		t.Fatal(err)
	}
	firstVolume, secondVolume := seeded.Volumes[0].ID, seeded.Volumes[1].ID
	var payload UploadInput
	var result UploadResult
	change := func(ctx context.Context) error {
		var err error
		result, err = st.Upload(ctx, payload)
		return err
	}
	// The result lists every chapter of the payload as it is stored, including
	// those the upload left alone.
	reported := func(orders ...int32) {
		t.Helper()
		if result.ChapterCount != len(orders) || len(result.Chapters) != len(orders) {
			t.Fatalf("upload result: %+v", result)
		}
		for i, order := range orders {
			got, chapter := s.chapter(t, novel.ID, order), result.Chapters[i]
			if chapter.ID != got.ID || chapter.Order != got.Order || !reflect.DeepEqual(chapter.VolumeID, got.VolumeID) || !chapter.UpdatedAt.Equal(got.UpdatedAt) {
				t.Fatalf("upload result does not describe chapter %d: %+v, stored %+v", order, chapter, got)
			}
		}
	}

	// The file holds the whole book as it was, with two chapters appended.
	payload = UploadInput{NovelID: &novel.ID, Volumes: []UploadVolume{{Order: 1, Title: "Volume one"}},
		Chapters: append(append([]UploadChapter{}, stored.Chapters...),
			UploadChapter{Order: 3, VolumeOrder: &one, Title: "Chapter three", Content: "Body three\n"},
			UploadChapter{Order: 4, Title: "Chapter four", Content: "Body four\n"})}
	baseline := s.rehearse(t, change)
	if got, err := st.ListChapters(ctx, novel.ID, 100, 0); err != nil || len(got) != 2 {
		t.Fatalf("rehearsal kept chapters: %+v %v", got, err)
	}
	// Meanwhile the volume is renamed, the first chapter rewritten and the
	// second moved to the other volume.
	if _, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 1, Title: "Volume one, renamed"}); err != nil {
		t.Fatal(err)
	}
	rewritten := s.save(t, ChapterInput{NovelID: novel.ID, VolumeID: &firstVolume, Order: 1, Title: "Chapter one", Content: "Body one, rewritten\n"})
	moved := s.save(t, ChapterInput{NovelID: novel.ID, VolumeID: &secondVolume, Order: 2, Title: "Chapter two", Content: "Body two\n"})
	changes := s.applied(t, baseline, change)
	if !reflect.DeepEqual(actions(changes), []string{"Chapter INSERT", "Chapter INSERT"}) {
		t.Fatalf("upload changed more than it appended: %+v", changes)
	}
	if got, err := st.GetVolume(ctx, novel.ID, 1); err != nil || got.Title != "Volume one, renamed" {
		t.Fatalf("volume rename lost: %+v %v", got, err)
	}
	if got := s.chapter(t, novel.ID, 1); got.Content != rewritten.Content || !got.UpdatedAt.Equal(rewritten.UpdatedAt) {
		t.Fatalf("rewritten chapter overwritten or touched: %+v", got)
	}
	if got := s.chapter(t, novel.ID, 2); got.VolumeID == nil || *got.VolumeID != secondVolume || !got.UpdatedAt.Equal(moved.UpdatedAt) {
		t.Fatalf("moved chapter put back or touched: %+v", got)
	}
	if got := s.chapter(t, novel.ID, 3); got.VolumeID == nil || *got.VolumeID != firstVolume || got.Content != "Body three\n" {
		t.Fatalf("appended chapter: %+v", got)
	}
	if got := s.chapter(t, novel.ID, 4); got.VolumeID != nil || got.Content != "Body four\n" {
		t.Fatalf("appended chapter: %+v", got)
	}
	if len(result.Volumes) != 1 || result.Volumes[0].Title != "Volume one, renamed" {
		t.Fatalf("upload result: %+v", result)
	}
	reported(1, 2, 3, 4)

	// A resent chapter with a new title keeps the body it was given since, one
	// with a new body keeps the title it was given since, and both stay in the
	// volume they were moved to.
	payload = UploadInput{NovelID: &novel.ID, Chapters: []UploadChapter{
		{Order: 1, VolumeOrder: &one, Title: "Chapter one", Content: "Body one, rewritten\n"},
		{Order: 2, VolumeOrder: &one, Title: "Chapter two, retitled", Content: "Body two\n"},
		{Order: 4, Title: "Chapter four", Content: "Body four, revised\n"},
	}}
	if _, err := st.SaveChapter(ctx, ChapterInput{NovelID: novel.ID, VolumeID: &firstVolume, Order: 2, Title: "Chapter two", Content: "Body two\n"}); err != nil {
		t.Fatal(err)
	}
	baseline = s.rehearse(t, change)
	s.save(t, ChapterInput{NovelID: novel.ID, VolumeID: &secondVolume, Order: 2, Title: "Chapter two", Content: "Body two, rewritten\n"})
	s.save(t, ChapterInput{NovelID: novel.ID, VolumeID: &secondVolume, Order: 4, Title: "Chapter four, retitled", Content: "Body four\n"})
	changes = s.applied(t, baseline, change)
	recorded := make(map[any]Change, len(changes))
	for _, c := range changes {
		recorded[c.After["order"]] = c
	}
	retitled, revised := recorded[float64(2)], recorded[float64(4)]
	if len(changes) != 2 || !reflect.DeepEqual(differing(retitled), []string{"title"}) || len(retitled.BodyDiff) != 0 ||
		!reflect.DeepEqual(differing(revised), []string{"contentHash", "contentLength"}) || len(revised.BodyDiff) == 0 {
		t.Fatalf("recorded more than a new title and a new body: %+v", changes)
	}
	if got := s.chapter(t, novel.ID, 2); got.Title != "Chapter two, retitled" || got.Content != "Body two, rewritten\n" || got.VolumeID == nil || *got.VolumeID != secondVolume {
		t.Fatalf("title not applied, or newer body or volume lost: %+v", got)
	}
	if got := s.chapter(t, novel.ID, 4); got.Title != "Chapter four, retitled" || got.Content != "Body four, revised\n" || got.VolumeID == nil || *got.VolumeID != secondVolume {
		t.Fatalf("body not applied, or newer title or volume lost: %+v", got)
	}
	reported(1, 2, 4)

	// A volume that only a chapter left alone refers to may be gone.
	payload = UploadInput{NovelID: &novel.ID, Chapters: []UploadChapter{
		{Order: 3, VolumeOrder: &one, Title: "Chapter three", Content: "Body three\n"},
		{Order: 5, Title: "Chapter five", Content: "Body five\n"},
	}}
	baseline = s.rehearse(t, change)
	for _, order := range []int32{1, 3} {
		chapter := s.chapter(t, novel.ID, order).ChapterInput
		chapter.VolumeID = nil
		s.save(t, chapter)
	}
	if err := st.DeleteVolume(ctx, novel.ID, 1); err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, change); !reflect.DeepEqual(actions(changes), []string{"Chapter INSERT"}) {
		t.Fatalf("upload changed more than it appended: %+v", changes)
	}
	if got := s.chapter(t, novel.ID, 3); got.VolumeID != nil {
		t.Fatalf("chapter put back into a removed volume: %+v", got)
	}

	// Appending is refused where someone else has appended in the meantime.
	payload = UploadInput{NovelID: &novel.ID, Chapters: []UploadChapter{
		{Order: 6, Title: "Chapter six", Content: "Body six\n"},
		{Order: 7, Title: "Chapter seven", Content: "Body seven\n"},
	}}
	baseline = s.rehearse(t, change)
	s.save(t, ChapterInput{NovelID: novel.ID, Order: 7, Title: "Someone else's chapter seven", Content: "Their body\n"})
	s.refused(t, baseline, change)
	if _, err := st.GetChapter(ctx, novel.ID, 6); !errors.Is(err, ErrNotFound) {
		t.Fatalf("refused upload kept a chapter: %v", err)
	}
	if got := s.chapter(t, novel.ID, 7); got.Title != "Someone else's chapter seven" {
		t.Fatalf("refused upload overwrote the other chapter: %+v", got)
	}
}

func (s *baselineSuite) uploadCreatingNovel(t *testing.T) {
	st, ctx := s.st, s.ctx
	one := int32(1)
	payload := UploadInput{Novel: &NovelInput{PlatformID: s.platform, Title: "Uploaded with a baseline", Status: "ONGOING", TagIDs: []int32{}},
		Volumes:  []UploadVolume{{Order: 1, Title: "Volume"}},
		Chapters: []UploadChapter{{Order: 1, VolumeOrder: &one, Title: "Chapter", Content: "Body\n"}}}
	var result UploadResult
	change := func(ctx context.Context) error {
		var err error
		result, err = st.Upload(ctx, payload)
		return err
	}
	baseline := s.rehearse(t, change)
	if _, err := st.GetNovel(ctx, result.Novel.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rehearsal kept the novel: %v", err)
	}
	// Nothing existed before, so there is nothing the upload could overwrite.
	var document map[string]any
	if err := json.Unmarshal(baseline, &document); err != nil || len(document) != 1 {
		t.Fatalf("baseline of a new novel lists something: %s %v", baseline, err)
	}
	if changes := s.applied(t, baseline, change); !reflect.DeepEqual(actions(changes), []string{"Chapter INSERT", "Novel INSERT", "Volume INSERT"}) {
		t.Fatalf("new novel not recorded: %+v", changes)
	}
	if got := s.chapter(t, result.Novel.ID, 1); got.Content != "Body\n" || got.VolumeID == nil {
		t.Fatalf("uploaded chapter: %+v", got)
	}
}

func (s *baselineSuite) volumes(t *testing.T) {
	st, ctx := s.st, s.ctx
	novel := s.novel(t, NovelInput{Title: "Volumes"})
	input := VolumeInput{NovelID: novel.ID, Order: 1, Title: "Volume"}
	if _, err := st.SaveVolume(ctx, input); err != nil {
		t.Fatal(err)
	}
	change := func(ctx context.Context) error { _, err := st.SaveVolume(ctx, input); return err }
	title := func(want string) {
		t.Helper()
		if got, err := st.GetVolume(ctx, novel.ID, 1); err != nil || got.Title != want {
			t.Fatalf("volume title %+v %v, want %q", got, err, want)
		}
	}

	// Saved as it was, a volume keeps a title it was given in the meantime.
	baseline := s.rehearse(t, change)
	if _, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 1, Title: "Renamed by someone else"}); err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, change); len(changes) != 0 {
		t.Fatalf("unchanged volume was changed: %+v", changes)
	}
	title("Renamed by someone else")

	// A new title is written over one the volume was given in the meantime.
	input.Title = "Renamed by the submitter"
	baseline = s.rehearse(t, change)
	if _, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 1, Title: "Renamed again by someone else"}); err != nil {
		t.Fatal(err)
	}
	changes := s.applied(t, baseline, change)
	if !reflect.DeepEqual(actions(changes), []string{"Volume UPDATE"}) || changes[0].Before["title"] != "Renamed again by someone else" {
		t.Fatalf("rename not recorded over the newer title: %+v", changes)
	}
	title(input.Title)

	// A volume that was removed is not brought back, and one that was removed
	// and created again is another volume.
	input.Title = "Renamed by the submitter again"
	baseline = s.rehearse(t, change)
	if err := st.DeleteVolume(ctx, novel.ID, 1); err != nil {
		t.Fatal(err)
	}
	s.refused(t, baseline, change)
	if _, err := st.GetVolume(ctx, novel.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("refused change brought a removed volume back: %v", err)
	}
	if _, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 1, Title: "Created again by someone else"}); err != nil {
		t.Fatal(err)
	}
	s.refused(t, baseline, change)
	title("Created again by someone else")

	// A new volume is refused when someone else has taken its place.
	added := VolumeInput{NovelID: novel.ID, Order: 2, Title: "Volume two"}
	add := func(ctx context.Context) error { _, err := st.SaveVolume(ctx, added); return err }
	baseline = s.rehearse(t, add)
	if _, err := st.GetVolume(ctx, novel.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rehearsal kept the volume: %v", err)
	}
	if _, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 2, Title: "Someone else's volume two"}); err != nil {
		t.Fatal(err)
	}
	s.refused(t, baseline, add)
	if got, err := st.GetVolume(ctx, novel.ID, 2); err != nil || got.Title != "Someone else's volume two" {
		t.Fatalf("refused change overwrote the other volume: %+v %v", got, err)
	}

	remove := func(ctx context.Context) error { return st.DeleteVolume(ctx, novel.ID, 1) }
	baseline = s.rehearse(t, remove)
	if _, err := st.SaveVolume(ctx, VolumeInput{NovelID: novel.ID, Order: 1, Title: "Kept by someone else"}); err != nil {
		t.Fatal(err)
	}
	s.refused(t, baseline, remove)
	title("Kept by someone else")
}

func (s *baselineSuite) catalogRecords(t *testing.T) {
	st, ctx := s.st, s.ctx
	tag, err := st.CreateTag(ctx, TagInput{Name: "baseline tag"})
	if err != nil {
		t.Fatal(err)
	}
	tagged := s.novel(t, NovelInput{Title: "Tagged", TagIDs: []int32{tag.ID}})
	other := s.novel(t, NovelInput{Title: "Tagged later"})
	name := TagInput{Name: "baseline tag, submitted"}
	rename := func(ctx context.Context) error { _, err := st.UpdateTag(ctx, tag.ID, name); return err }
	tagName := func(want string) {
		t.Helper()
		if got, err := st.GetTag(ctx, tag.ID); err != nil || got.Name != want {
			t.Fatalf("tag %+v %v, want %q", got, err, want)
		}
	}

	// A tag that more novels carry by now is still the tag that was renamed.
	baseline := s.rehearse(t, rename)
	tagName("baseline tag")
	if _, err := st.UpdateNovel(ctx, other.ID, NovelInput{PlatformID: s.platform, Title: other.Title, Status: "ONGOING", TagIDs: []int32{tag.ID}}); err != nil {
		t.Fatal(err)
	}
	changes := s.applied(t, baseline, rename)
	tagName(name.Name)
	// History lists the novels that show the tag, as it always did, and never
	// the tag itself.
	if !reflect.DeepEqual(actions(changes), []string{"Novel UPDATE", "Novel UPDATE"}) {
		t.Fatalf("tag rename history: %+v", changes)
	}
	for _, change := range changes {
		if !reflect.DeepEqual(differing(change), []string{"tagNames"}) {
			t.Fatalf("tag rename history: %+v", change)
		}
	}
	for _, drafts := range s.recorded[len(s.recorded)-1:] {
		if len(drafts) != 2 || drafts[0].NovelID != tagged.ID || drafts[1].NovelID != other.ID {
			t.Fatalf("tag rename history: %+v", drafts)
		}
	}

	// A new name is written over one the tag was given in the meantime: of two
	// renames the one applied later stands.
	name.Name = "baseline tag, submitted again"
	baseline = s.rehearse(t, rename)
	if _, err := st.UpdateTag(ctx, tag.ID, TagInput{Name: "baseline tag, by someone else"}); err != nil {
		t.Fatal(err)
	}
	changes = s.applied(t, baseline, rename)
	tagName(name.Name)
	if len(changes) != 2 {
		t.Fatalf("tag rename history: %+v", changes)
	}
	for _, change := range changes {
		if !reflect.DeepEqual(differing(change), []string{"tagNames"}) || !reflect.DeepEqual(change.Before["tagNames"], []any{"baseline tag, by someone else"}) {
			t.Fatalf("tag rename not recorded over the newer name: %+v", change)
		}
	}

	// Removing a tag is refused once the tag is no longer what was removed.
	unused, err := st.CreateTag(ctx, TagInput{Name: "baseline unused tag"})
	if err != nil {
		t.Fatal(err)
	}
	remove := func(ctx context.Context) error { return st.DeleteTag(ctx, unused.ID) }
	baseline = s.rehearse(t, remove)
	if _, err := st.UpdateTag(ctx, unused.ID, TagInput{Name: "baseline tag in use after all"}); err != nil {
		t.Fatal(err)
	}
	s.refused(t, baseline, remove)
	if got, err := st.GetTag(ctx, unused.ID); err != nil || got.Name != "baseline tag in use after all" {
		t.Fatalf("refused removal altered the tag: %+v %v", got, err)
	}
	baseline = s.rehearse(t, remove)
	if changes := s.applied(t, baseline, remove); len(changes) != 0 {
		t.Fatalf("tag removal reached history: %+v", changes)
	}
	if _, err := st.GetTag(ctx, unused.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("tag not removed: %v", err)
	}

	penName, submitted, theirs := "Baseline author", "Baseline author, submitted", "Baseline author, by someone else"
	author, err := st.CreateAuthor(ctx, AuthorInput{PlatformID: s.platform, Name: &penName})
	if err != nil {
		t.Fatal(err)
	}
	signed := s.novel(t, NovelInput{Title: "Signed", AuthorID: &author.ID})
	sign := func(ctx context.Context) error { _, err := st.UpdateAuthorName(ctx, author.ID, &submitted); return err }
	// The new name of an author is written over a newer one in the same way.
	baseline = s.rehearse(t, sign)
	if got, err := st.GetAuthor(ctx, author.ID); err != nil || got.Name == nil || *got.Name != penName {
		t.Fatalf("rehearsal renamed the author: %+v %v", got, err)
	}
	if _, err := st.UpdateAuthorName(ctx, author.ID, &theirs); err != nil {
		t.Fatal(err)
	}
	changes = s.applied(t, baseline, sign)
	if len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"authorName"}) || changes[0].Before["authorName"] != theirs ||
		s.recorded[len(s.recorded)-1][0].NovelID != signed.ID {
		t.Fatalf("author rename history: %+v", changes)
	}
	if got, err := st.GetAuthor(ctx, author.ID); err != nil || got.Name == nil || *got.Name != submitted {
		t.Fatalf("author not renamed: %+v %v", got, err)
	}
}

func (s *baselineSuite) illustrationDeletion(t *testing.T) {
	st, ctx := s.st, s.ctx
	novel := s.novel(t, NovelInput{Title: "Illustrated"})
	var stored [2]Illustration
	for i, digest := range []string{"c", "d"} {
		image, err := st.AddIllustration(ctx, novel.ID, IllustrationInput{
			URL: "https://images.example.test/" + digest + ".jpg", SHA256: strings.Repeat(digest, 64), Width: 320, Height: 240,
		})
		if err != nil {
			t.Fatal(err)
		}
		stored[i] = image
	}
	layout, err := st.SaveIllustrationLayout(ctx, novel.ID, IllustrationLayout{Blocks: []IllustrationBlock{
		{ImageID: stored[0].ID, X: 0, Y: 0, Width: 0.5, Height: 0.25},
		{ImageID: stored[1].ID, X: 0.5, Y: 0, Width: 0.5, Height: 0.25},
	}})
	if err != nil {
		t.Fatal(err)
	}
	remove := func(id int32) func(context.Context) error {
		return func(ctx context.Context) error { return st.DeleteIllustration(ctx, novel.ID, id) }
	}
	images := func(want int) {
		t.Helper()
		if got, err := st.ListIllustrations(ctx, novel.ID, 100, 0); err != nil || len(got) != want {
			t.Fatalf("images %+v %v, want %d", got, err, want)
		}
	}

	// Removing an image rewrites the layout that shows it. The image is taken
	// out of the layout as it is by now, however it was rearranged in the
	// meantime.
	baseline := s.rehearse(t, remove(stored[0].ID))
	images(2)
	rearranged, err := st.SaveIllustrationLayout(ctx, novel.ID, IllustrationLayout{Revision: layout.Revision, Blocks: []IllustrationBlock{
		{ImageID: stored[1].ID, X: 0, Y: 0, Width: 1, Height: 0.5},
		{ImageID: stored[0].ID, X: 0, Y: 0.5, Width: 1, Height: 0.5},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, remove(stored[0].ID)); !reflect.DeepEqual(actions(changes), []string{"Illustration DELETE", "IllustrationLayout UPDATE"}) {
		t.Fatalf("removal not recorded: %+v", changes)
	}
	current, err := st.GetIllustrationLayout(ctx, novel.ID)
	if err != nil || current.Revision != rearranged.Revision+1 || len(current.Blocks) != 1 || current.Blocks[0].ImageID != stored[1].ID || current.Blocks[0].Height != 0.5 {
		t.Fatalf("layout after removal: %+v %v", current, err)
	}
	images(1)

	// An image that the layout did not show when its removal was submitted, and
	// shows by now, stays: removing it would rewrite a layout the submitter
	// never meant to touch.
	placed, err := st.AddIllustration(ctx, novel.ID, IllustrationInput{
		URL: "https://images.example.test/f.jpg", SHA256: strings.Repeat("f", 64), Width: 320, Height: 240,
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline = s.rehearse(t, remove(placed.ID))
	shown, err := st.SaveIllustrationLayout(ctx, novel.ID, IllustrationLayout{Revision: current.Revision, Blocks: append(current.Blocks,
		IllustrationBlock{ImageID: placed.ID, X: 0, Y: 0.5, Width: 1, Height: 0.5})})
	if err != nil {
		t.Fatal(err)
	}
	s.refused(t, baseline, remove(placed.ID))
	if got, err := st.GetIllustrationLayout(ctx, novel.ID); err != nil || got.Revision != shown.Revision || len(got.Blocks) != 2 {
		t.Fatalf("refused removal altered the layout: %+v %v", got, err)
	}
	images(2)

	// The other way round, an image that was taken out of the layout in the
	// meantime is removed, and the layout stays as it is.
	baseline = s.rehearse(t, remove(placed.ID))
	cleared, err := st.SaveIllustrationLayout(ctx, novel.ID, IllustrationLayout{Revision: shown.Revision, Blocks: current.Blocks})
	if err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, remove(placed.ID)); !reflect.DeepEqual(actions(changes), []string{"Illustration DELETE"}) {
		t.Fatalf("removal not recorded on its own: %+v", changes)
	}
	if got, err := st.GetIllustrationLayout(ctx, novel.ID); err != nil || got.Revision != cleared.Revision || len(got.Blocks) != 1 || got.Blocks[0].ImageID != stored[1].ID {
		t.Fatalf("removal altered a layout that no longer showed the image: %+v %v", got, err)
	}
	images(1)

	// An image is added once, whoever adds it first.
	input := IllustrationInput{URL: "https://images.example.test/e.jpg", SHA256: strings.Repeat("e", 64), Width: 320, Height: 240}
	add := func(ctx context.Context) error { _, err := st.AddIllustration(ctx, novel.ID, input); return err }
	baseline = s.rehearse(t, add)
	if _, err := st.AddIllustration(ctx, novel.ID, input); err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, add); len(changes) != 0 {
		t.Fatalf("image that is already there was changed: %+v", changes)
	}
}

func (s *baselineSuite) illustrationLayouts(t *testing.T) {
	st, ctx := s.st, s.ctx
	novel := s.novel(t, NovelInput{Title: "Laid out"})
	var stored [3]Illustration
	for i, digest := range []string{"1", "2", "3"} {
		image, err := st.AddIllustration(ctx, novel.ID, IllustrationInput{
			URL: "https://images.example.test/layout-" + digest + ".jpg", SHA256: strings.Repeat(digest, 64), Width: 320, Height: 240,
		})
		if err != nil {
			t.Fatal(err)
		}
		stored[i] = image
	}
	block := func(image Illustration, y float64) IllustrationBlock {
		return IllustrationBlock{ImageID: image.ID, X: 0, Y: y, Width: 400, Height: 300}
	}
	// What a baseline keeps of a block.
	kept := func(image Illustration, y float64) map[string]any {
		return map[string]any{"imageId": float64(image.ID), "x": 0.0, "y": y, "width": 400.0, "height": 300.0}
	}
	var input, saved IllustrationLayout
	change := func(ctx context.Context) error {
		var err error
		saved, err = st.SaveIllustrationLayout(ctx, novel.ID, input)
		return err
	}
	layout := func() IllustrationLayout {
		t.Helper()
		got, err := st.GetIllustrationLayout(ctx, novel.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	// direct stores a layout the way someone else does while a change is waiting.
	direct := func(blocks ...IllustrationBlock) IllustrationLayout {
		t.Helper()
		got, err := st.SaveIllustrationLayout(ctx, novel.ID, IllustrationLayout{Revision: layout().Revision, Blocks: blocks})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// The first layout of a novel is written over one that was stored in the
	// meantime: of two layouts the one applied later stands.
	input = IllustrationLayout{Blocks: []IllustrationBlock{block(stored[0], 0)}}
	baseline := s.rehearse(t, change)
	if got := layout(); got.Revision != 0 || len(got.Blocks) != 0 {
		t.Fatalf("rehearsal stored the layout: %+v", got)
	}
	// The baseline tells what the layout becomes, for showing it while it waits.
	queued, err := SubmittedChanges(baseline, novel.ID)
	if want := []SubmittedChange{{Entity: "IllustrationLayout", Action: "INSERT",
		After: map[string]any{"novelId": float64(novel.ID), "blocks": []any{kept(stored[0], 0)}}}}; err != nil || !reflect.DeepEqual(queued, want) {
		t.Fatalf("queued layout: %+v %v, want %+v", queued, err, want)
	}
	theirs := direct(block(stored[1], 0))
	changes := s.applied(t, baseline, change)
	if !reflect.DeepEqual(actions(changes), []string{"IllustrationLayout UPDATE"}) || !reflect.DeepEqual(differing(changes[0]), []string{"blocks"}) {
		t.Fatalf("layout not recorded: %+v", changes)
	}
	if got := layout(); got.Revision != theirs.Revision+1 || !reflect.DeepEqual(got.Blocks, input.Blocks) || !reflect.DeepEqual(saved, got) {
		t.Fatalf("layout not written over the newer one: %+v, saved %+v", got, saved)
	}

	// Two layouts that were submitted for the same revision are both applied,
	// in the order they are approved in.
	from := layout()
	first := IllustrationLayout{Revision: from.Revision, Blocks: []IllustrationBlock{block(stored[1], 0), block(stored[2], 300)}}
	second := IllustrationLayout{Revision: from.Revision, Blocks: []IllustrationBlock{block(stored[2], 0)}}
	input = first
	firstBaseline := s.rehearse(t, change)
	input = second
	secondBaseline := s.rehearse(t, change)
	if got := layout(); !reflect.DeepEqual(got, from) {
		t.Fatalf("rehearsal stored the layout: %+v", got)
	}
	queued, err = SubmittedChanges(secondBaseline, novel.ID)
	if want := []SubmittedChange{{Entity: "IllustrationLayout", Action: "UPDATE", Fields: []string{"blocks"},
		After: map[string]any{"blocks": []any{kept(stored[2], 0)}}}}; err != nil || !reflect.DeepEqual(queued, want) {
		t.Fatalf("queued layout: %+v %v, want %+v", queued, err, want)
	}
	input = first
	if changes := s.applied(t, firstBaseline, change); !reflect.DeepEqual(actions(changes), []string{"IllustrationLayout UPDATE"}) {
		t.Fatalf("layout not recorded: %+v", changes)
	}
	if got := layout(); got.Revision != from.Revision+1 || !reflect.DeepEqual(got.Blocks, first.Blocks) {
		t.Fatalf("first layout not applied: %+v", got)
	}
	input = second
	if changes := s.applied(t, secondBaseline, change); !reflect.DeepEqual(actions(changes), []string{"IllustrationLayout UPDATE"}) {
		t.Fatalf("layout not recorded: %+v", changes)
	}
	if got := layout(); got.Revision != from.Revision+2 || !reflect.DeepEqual(got.Blocks, second.Blocks) {
		t.Fatalf("second layout not written over the first: %+v", got)
	}

	// The revision still tells a submitter that the layout it worked from has
	// been replaced: an outdated layout is turned down when it is submitted,
	// not while it waits.
	input = IllustrationLayout{Revision: from.Revision, Blocks: []IllustrationBlock{block(stored[0], 0)}}
	if _, err := st.SaveIllustrationLayout(ctx, novel.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("outdated layout stored: %v", err)
	}
	rehearsing, rehearsal := WithRehearsal(ctx)
	if err := st.Mutate(rehearsing, change); !errors.Is(err, ErrConflict) || rehearsal.Baseline() != nil {
		t.Fatalf("outdated layout rehearsed: %v, baseline %s", err, rehearsal.Baseline())
	}

	// A layout that was submitted as it stood alters nothing when it is
	// applied, whatever was stored in the meantime.
	input = layout()
	baseline = s.rehearse(t, change)
	if queued, err := SubmittedChanges(baseline, novel.ID); err != nil || len(queued) != 0 {
		t.Fatalf("unchanged layout is listed as a change: %+v %v", queued, err)
	}
	theirs = direct(block(stored[0], 0), block(stored[1], 300))
	if changes := s.applied(t, baseline, change); len(changes) != 0 {
		t.Fatalf("unchanged layout was changed: %+v", changes)
	}
	if got := layout(); !reflect.DeepEqual(got, theirs) || !reflect.DeepEqual(saved, got) {
		t.Fatalf("unchanged layout written over the newer one: %+v, saved %+v", got, saved)
	}

	// A layout cannot show an image that was removed while it waited. Such a
	// change is not outdated but impossible, and fails as that.
	input = IllustrationLayout{Revision: theirs.Revision, Blocks: []IllustrationBlock{block(stored[2], 0)}}
	baseline = s.rehearse(t, change)
	if err := st.DeleteIllustration(ctx, novel.ID, stored[2].ID); err != nil {
		t.Fatal(err)
	}
	recorded := len(s.recorded)
	if err := s.approve(t, baseline, change); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("layout showing a removed image: %v", err)
	}
	if len(s.recorded) != recorded {
		t.Fatal("failed layout recorded a revision")
	}
	if got := layout(); !reflect.DeepEqual(got, theirs) {
		t.Fatalf("failed layout altered the stored one: %+v", got)
	}
}

func (s *baselineSuite) documents(t *testing.T) {
	st, ctx := s.st, s.ctx
	form := NovelInput{PlatformID: s.platform, Title: "Document", Status: "ONGOING", TagIDs: []int32{}}
	novel := s.novel(t, form)
	edit := form
	edit.Title = "Document, edited"
	change := func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, novel.ID, edit); return err }
	title := func(want string) {
		t.Helper()
		if got, err := st.GetNovel(ctx, novel.ID); err != nil || got.Title != want {
			t.Fatalf("novel title %+v %v, want %q", got, err, want)
		}
	}

	// A baseline that cannot be read says nothing about the content: the
	// change fails, and not as an outdated one.
	for _, document := range []string{`{"v":1,"novels":`, `[]`, `"baseline"`, `{}`, `{"v":2}`, `{"v":1,"novels":{"first":[]}}`} {
		recorded := len(s.recorded)
		if err := st.Mutate(WithBaseline(ctx, []byte(document)), change); err == nil || errors.Is(err, ErrStale) {
			t.Fatalf("baseline %s: %v", document, err)
		}
		if len(s.recorded) != recorded {
			t.Fatalf("baseline %s: change recorded a revision", document)
		}
		title(form.Title)
	}

	// A document without content is no baseline at all.
	for _, document := range []string{"", "null", " null\n"} {
		if err := st.Mutate(WithBaseline(ctx, []byte(document)), func(ctx context.Context) error {
			if _, tracked := submitted(ctx, novel.ID); tracked {
				t.Errorf("baseline %q was attached", document)
			}
			return nil
		}); err != nil {
			t.Fatalf("baseline %q: %v", document, err)
		}
	}

	// A failed rehearsal has nothing to say either.
	rehearsing, rehearsal := WithRehearsal(ctx)
	if err := st.Mutate(rehearsing, func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, novel.ID+100000, edit); return err }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rehearsal of an impossible change: %v", err)
	}
	if rehearsal.Baseline() != nil {
		t.Fatalf("failed rehearsal left a baseline: %s", rehearsal.Baseline())
	}

	// The host may rewrite the document as long as it means the same.
	rewrite := func(baseline []byte) []byte {
		t.Helper()
		var document map[string]any
		if err := json.Unmarshal(baseline, &document); err != nil {
			t.Fatal(err)
		}
		rewritten, err := json.MarshalIndent(document, "", "    ")
		if err != nil || bytes.Equal(rewritten, baseline) {
			t.Fatalf("baseline was not rewritten: %s %v", rewritten, err)
		}
		// Object keys now come in another order than the one they were written in.
		if bytes.Index(rewritten, []byte(`"novels"`)) > bytes.Index(rewritten, []byte(`"v"`)) || bytes.Index(rewritten, []byte(`"action"`)) > bytes.Index(rewritten, []byte(`"key"`)) {
			t.Fatalf("baseline keys kept their order: %s", rewritten)
		}
		return rewritten
	}
	// A removal is compared with what the baseline kept, value by value, and
	// the rewritten document still compares equal to the record it describes.
	chapter := ChapterInput{NovelID: novel.ID, Order: 1, Title: "Chapter", Content: "Body\n"}
	s.save(t, chapter)
	remove := func(ctx context.Context) error { return st.DeleteChapter(ctx, novel.ID, 1) }
	baseline := rewrite(s.rehearse(t, remove))
	chapter.Title = "Chapter, retitled by someone else"
	s.save(t, chapter)
	s.refused(t, baseline, remove)
	if got := s.chapter(t, novel.ID, 1); got.Title != chapter.Title {
		t.Fatalf("refused removal altered the chapter: %+v", got)
	}
	baseline = rewrite(s.rehearse(t, remove))
	if changes := s.applied(t, baseline, remove); !reflect.DeepEqual(actions(changes), []string{"Chapter DELETE"}) {
		t.Fatalf("rewritten baseline of a removal: %+v", changes)
	}

	// An edit applied with a rewritten baseline writes what it altered, and
	// leaves alone what it did not.
	baseline = rewrite(s.rehearse(t, change))
	cover := "https://images.example.test/document.jpg"
	recovered := form
	recovered.CoverURL = &cover
	if _, err := st.UpdateNovel(ctx, novel.ID, recovered); err != nil {
		t.Fatal(err)
	}
	if changes := s.applied(t, baseline, change); len(changes) != 1 || !reflect.DeepEqual(differing(changes[0]), []string{"title"}) {
		t.Fatalf("rewritten baseline: %+v", changes)
	}
	title(edit.Title)
	if got, err := st.GetNovel(ctx, novel.ID); err != nil || got.CoverURL == nil || *got.CoverURL != cover {
		t.Fatalf("newer cover lost: %+v %v", got, err)
	}
}

func (s *baselineSuite) receipts(t *testing.T) {
	st, ctx := s.st, s.ctx
	novel := s.novel(t, NovelInput{Title: "Receipt"})
	stored := ChapterInput{NovelID: novel.ID, Order: 1, Title: "Chapter", Content: "Body\n"}
	s.save(t, stored)
	edit := stored
	edit.Title = "Chapter, retitled"
	change := func(ctx context.Context) error { _, err := st.SaveChapter(ctx, edit); return err }
	receipts := func(id string) (count int) {
		t.Helper()
		if err := st.pool.QueryRow(ctx, `SELECT count(*) FROM "MutationReceipt" WHERE id=$1::uuid`, id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	approve := func(id string, baseline []byte) error {
		approval, receipt := NewMutationReceipt(WithMutationID(WithBaseline(ctx, baseline), id))
		if receipt == nil {
			t.Fatal("approval without a receipt")
		}
		return st.Mutate(approval, change)
	}

	// A refused change may be decided again: it left no receipt behind. The
	// chapter it retitles was removed in the meantime.
	baseline := s.rehearse(t, change)
	if err := st.DeleteChapter(ctx, novel.ID, 1); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	recorded := len(s.recorded)
	if err := approve(id, baseline); !errors.Is(err, ErrStale) {
		t.Fatalf("outdated change not refused: %v", err)
	}
	if receipts(id) != 0 || len(s.recorded) != recorded {
		t.Fatal("refused change left a receipt or a revision")
	}

	// An applied change answers a repeated approval from its receipt, and is
	// not written over content that has moved on since.
	s.save(t, stored)
	baseline = s.rehearse(t, change)
	if err := approve(id, baseline); err != nil || receipts(id) != 1 || len(s.recorded) != recorded+1 {
		t.Fatalf("change not applied once: %v", err)
	}
	if got := s.chapter(t, novel.ID, 1); got.Title != edit.Title {
		t.Fatalf("change not applied: %+v", got)
	}
	retitled := stored
	retitled.Title = "Chapter, retitled by someone else"
	s.save(t, retitled)
	if err := approve(id, baseline); err != nil || len(s.recorded) != recorded+1 {
		t.Fatalf("repeated approval: %v", err)
	}
	if got := s.chapter(t, novel.ID, 1); got.Title != retitled.Title {
		t.Fatalf("repeated approval applied the change again: %+v", got)
	}
}
