package storage

import (
	"context"
	"testing"
)

func testWordCount(t *testing.T, ctx context.Context, st *Postgres) {
	platform, err := st.CreatePlatform(ctx, PlatformInput{Name: "word count"})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, err := st.Upload(ctx, UploadInput{
		Novel: &NovelInput{PlatformID: platform.ID, Title: "统计测试", Status: "ONGOING"},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := uploaded.Novel.ID
	check := func(want int64) {
		t.Helper()
		toc, err := st.GetTOC(ctx, id)
		if err != nil || toc.WordCount != want {
			t.Fatalf("word count = %d, want %d; err=%v", toc.WordCount, want, err)
		}
	}
	check(0)
	// Chinese, Latin, punctuation and supplementary-plane characters count
	// once each; spaces and newlines do not count. Titles are not added.
	if _, err := st.SaveChapter(ctx, ChapterInput{NovelID: id, Order: 1, Title: "标题不另计", Content: "中文 A😀。\n\t\u00a0\u1680\u2003\u2028\u2029\u202f\u205f\u3000\ufeff"}); err != nil {
		t.Fatal(err)
	}
	check(5)
	if _, err := st.Upload(ctx, UploadInput{NovelID: &id, Chapters: []UploadChapter{{Order: 2, Title: "第二章", Content: "正文\n内容"}}}); err != nil {
		t.Fatal(err)
	}
	check(9)
	if _, err := st.SaveChapter(ctx, ChapterInput{NovelID: id, Order: 1, Title: "已改", Content: "新内容"}); err != nil {
		t.Fatal(err)
	}
	check(7)
	if err := st.DeleteChapter(ctx, id, 2); err != nil {
		t.Fatal(err)
	}
	check(3)
}
