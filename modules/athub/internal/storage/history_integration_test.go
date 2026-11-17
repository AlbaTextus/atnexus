package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func testHistoryIntegration(t *testing.T, ctx context.Context, st *Postgres) {
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "history"})
	if err != nil {
		t.Fatal(err)
	}
	var batches [][]RevisionDraft
	var finalized []bool
	st.RecordChanges = func(_ context.Context, drafts []RevisionDraft) (func(context.Context, bool) error, error) {
		batches = append(batches, drafts)
		return func(_ context.Context, committed bool) error { finalized = append(finalized, committed); return nil }, nil
	}
	defer func() { st.RecordChanges = nil }()
	ctx = WithContributor(ctx, Contributor{ID: "account-id", Name: "贡献者"})
	var novel Novel
	input := NovelInput{PlatformID: platform.ID, Title: "History", Status: "ONGOING"}
	if err := st.Mutate(ctx, func(ctx context.Context) error {
		var err error
		novel, err = st.CreateNovel(ctx, input)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0]) != 1 || len(batches[0][0].Changes) != 1 ||
		batches[0][0].Contributor.ID != "account-id" || batches[0][0].Changes[0].Action != "INSERT" ||
		len(finalized) != 1 || !finalized[0] {
		t.Fatalf("creation not attributed/grouped: %+v", batches)
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, novel.ID, input); return err }); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatal("no-op created history")
	}
	order := int32(1)
	if err := st.Mutate(ctx, func(ctx context.Context) error {
		_, err := st.Upload(ctx, UploadInput{NovelID: &novel.ID, Volumes: []UploadVolume{{Order: 1, Title: "Volume"}},
			Chapters: []UploadChapter{{Order: 1, VolumeOrder: &order, Title: "Chapter", Content: "private chapter body"}}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || len(batches[1]) != 1 || len(batches[1][0].Changes) != 2 {
		t.Fatalf("upload not one revision: %+v", batches)
	}
	for _, change := range batches[1][0].Changes {
		if _, ok := change.After["content"]; ok {
			t.Fatal("history copied chapter body")
		}
		if change.Entity == "Chapter" && change.After["contentHash"] == nil {
			t.Fatal("missing body change marker")
		}
	}
	rollback := errors.New("rollback")
	if err := st.Mutate(ctx, func(ctx context.Context) error {
		input.Title = "Must roll back"
		_, err := st.UpdateNovel(ctx, novel.ID, input)
		if err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got, err := st.GetNovel(ctx, novel.ID); err != nil || got.Title != "History" || len(batches) != 2 {
		t.Fatal("failed mutation changed content/history")
	}
	st.RecordChanges = func(context.Context, []RevisionDraft) (func(context.Context, bool) error, error) {
		return nil, errors.New("history offline")
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error { _, err := st.UpdateNovel(ctx, novel.ID, input); return err }); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal("history outage allowed unrecorded write")
	}
	if got, err := st.GetNovel(ctx, novel.ID); err != nil || got.Title != "History" {
		t.Fatal("history outage failed to roll back content")
	}
	// Newly uploaded novels must be attributed too.
	st.RecordChanges = func(_ context.Context, drafts []RevisionDraft) (func(context.Context, bool) error, error) {
		if len(drafts) != 1 || drafts[0].NovelID <= 0 {
			t.Fatal("new upload missing novel")
		}
		batches = append(batches, drafts)
		return func(context.Context, bool) error { return nil }, nil
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error { _, err := st.Upload(ctx, UploadInput{Novel: &input}); return err }); err != nil {
		t.Fatal(err)
	}
	t.Run("chapter body revisions", func(t *testing.T) { testChapterBodyRevisions(t, ctx, st, platform.ID) })
}

func testChapterBodyRevisions(t *testing.T, ctx context.Context, st *Postgres, platformID int32) {
	previousRecorder := st.RecordChanges
	defer func() { st.RecordChanges = previousRecorder }()
	novel, err := st.CreateNovel(ctx, NovelInput{PlatformID: platformID, Title: "Body history", Status: "ONGOING"})
	if err != nil {
		t.Fatal(err)
	}
	chapter := ChapterInput{NovelID: novel.ID, Order: 1, Title: "正文变化", Content: "第一行\n她是一只猫🐱。\n最后一行"}
	original := chapter.Content
	if _, err := st.SaveChapter(ctx, chapter); err != nil {
		t.Fatal(err)
	}
	var batches [][]RevisionDraft
	st.RecordChanges = func(_ context.Context, drafts []RevisionDraft) (func(context.Context, bool) error, error) {
		batches = append(batches, drafts)
		return func(context.Context, bool) error { return nil }, nil
	}
	chapter.Content = "第一行\n她是一只猫娘🐱。\n最后一行"
	if err := st.Mutate(ctx, func(ctx context.Context) error { _, err := st.SaveChapter(ctx, chapter); return err }); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0][0].Changes) != 1 {
		t.Fatal("body update not recorded")
	}
	change := batches[0][0].Changes[0]
	if change.Entity != "Chapter" || len(change.BodyDiff) == 0 || reconstructBody(t, original, change.BodyDiff) != chapter.Content {
		t.Fatalf("body diff not retained: %+v", change)
	}
	if _, ok := change.After["content"]; ok {
		t.Fatal("complete body snapshot should not be copied")
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error { _, err := st.SaveChapter(ctx, chapter); return err }); err != nil || len(batches) != 1 {
		t.Fatal("body no-op recorded a revision", err)
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error {
		_, err := st.Upload(ctx, UploadInput{NovelID: &novel.ID, Chapters: []UploadChapter{{Order: 1, Title: chapter.Title, Content: original}}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || reconstructBody(t, chapter.Content, batches[1][0].Changes[0].BodyDiff) != original {
		t.Fatal("bulk upload missing body diff")
	}
	rollback := errors.New("cancel body change")
	if err := st.Mutate(ctx, func(ctx context.Context) error {
		if _, err := st.SaveChapter(ctx, chapter); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got, err := st.GetChapter(ctx, novel.ID, 1); err != nil || got.Content != original || len(batches) != 2 {
		t.Fatal("failed request changed body or history")
	}
	st.RecordChanges = func(context.Context, []RevisionDraft) (func(context.Context, bool) error, error) {
		return nil, errors.New("body history offline")
	}
	if err := st.Mutate(ctx, func(ctx context.Context) error { _, err := st.SaveChapter(ctx, chapter); return err }); err == nil {
		t.Fatal("body write allowed during history outage")
	}
	if got, err := st.GetChapter(ctx, novel.ID, 1); err != nil || got.Content != original {
		t.Fatal("history outage failed to roll back body")
	}
}

func reconstructBody(t *testing.T, before string, hunks []BodyHunk) string {
	t.Helper()
	source := []rune(before)
	var result []rune
	cursor := 0
	for _, hunk := range hunks {
		var old, next string
		for _, part := range hunk.Parts {
			if part.Kind != "insert" {
				old += part.Text
			}
			if part.Kind != "delete" {
				next += part.Text
			}
		}
		end := hunk.BeforeStart + len([]rune(old))
		if hunk.BeforeStart < cursor || end > len(source) || string(source[hunk.BeforeStart:end]) != old {
			t.Fatalf("invalid original fragment: %+v", hunk)
		}
		result = append(result, source[cursor:hunk.BeforeStart]...)
		if len(result) != hunk.AfterStart || hunk.BeforeLine != 1+strings.Count(string(source[:hunk.BeforeStart]), "\n") || hunk.AfterLine != 1+strings.Count(string(result), "\n") {
			t.Fatalf("invalid Unicode position or line number: %+v", hunk)
		}
		result = append(result, []rune(next)...)
		cursor = end
	}
	result = append(result, source[cursor:]...)
	return string(result)
}

func TestChapterBodyDiff(t *testing.T) {
	long := strings.Repeat("未修改的内容😀\n", 1000)
	for _, test := range []struct{ name, before, after string }{
		{"no-op", "原文", "原文"},
		{"insert", "", "新正文🐱"},
		{"delete", "删除正文🐱", ""},
		{"unicode", "她是一只猫🐱。\n下一行", "她是一只猫娘🐱。\n下一行"},
		{"crlf", "第一行\r\n第二行\r\n", "首行\r\n第二行\r\n末行"},
		{"distant changes", "开头\n" + long + "结尾", "新开头\n" + long + "新结尾"},
		{"same length", "春风又绿江南岸", "春风又到江南岸"},
		{"replacement", "原来的段落", "<script>另一段</script>"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hunks := chapterBodyDiff(test.before, test.after, chapterDiffTimeout)
			if got := reconstructBody(t, test.before, hunks); got != test.after {
				t.Fatalf("reconstructed %q, want %q", got, test.after)
			}
			if test.before == test.after && len(hunks) != 0 {
				t.Fatal("no-op produced body hunks")
			}
			if test.name == "distant changes" {
				if len(hunks) != 2 {
					t.Fatal("distant changes should retain two small fragments")
				}
				for _, hunk := range hunks {
					for _, part := range hunk.Parts {
						if part.Kind == "equal" && len([]rune(part.Text)) > 96 {
							t.Fatal("unchanged chapter content copied to history")
						}
					}
				}
			}
		})
	}
}
