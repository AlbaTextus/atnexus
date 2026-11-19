package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/modules/athub/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUploadIntegration(t *testing.T) {
	dsn := os.Getenv("ATHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ATHUB_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	schema := pgx.Identifier{fmt.Sprintf("athub_api_test_%d", time.Now().UnixNano())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	paths, err := filepath.Glob("../../../../prisma/athub/migrations/*/migration.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("migrations: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := strings.Replace(string(data), `CREATE SCHEMA IF NOT EXISTS "public";`, "", 1)
		if _, err := pool.Exec(ctx, sql, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
	}
	store := storage.NewPostgres(pool)
	handler := testHandler(t, store)
	call := func(method, path string, body any, status int, target any) {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		w := request(handler, method, path, string(data))
		if w.Code != status {
			t.Fatalf("%s %s: want %d got %d: %s", method, path, status, w.Code, w.Body.String())
		}
		if target != nil {
			if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
				t.Fatal(err)
			}
		}
	}
	var platform storage.Platform
	call("POST", "/api/v2/platforms", storage.PlatformInput{Name: " Local "}, 201, &platform)
	if platform.Name != "Local" || platform.URL != nil {
		t.Fatalf("platform: %+v", platform)
	}
	var platforms []storage.Platform
	call("GET", "/api/v2/platforms?limit=1", nil, 200, &platforms)
	if len(platforms) != 1 || platforms[0].ID != platform.ID {
		t.Fatalf("platforms: %+v", platforms)
	}
	call("GET", fmt.Sprintf("/api/v2/platforms?id=%d", platform.ID), nil, 200, &platform)
	call("GET", "/api/v2/platforms?id=2147483647", nil, 404, nil)
	var author storage.Author
	call("POST", "/api/v2/authors", storage.AuthorInput{PlatformID: platform.ID}, 201, &author)
	call("GET", fmt.Sprintf("/api/v2/authors?id=%d", author.ID), nil, 200, &author)
	var authors []storage.Author
	call("GET", fmt.Sprintf("/api/v2/authors?platformId=%d", platform.ID), nil, 200, &authors)
	if len(authors) != 1 || authors[0].ID != author.ID {
		t.Fatalf("authors: %+v", authors)
	}
	call("PUT", fmt.Sprintf("/api/v2/authors?id=%d", author.ID), map[string]any{"name": "  真实笔名  "}, 200, &author)
	if author.Name == nil || *author.Name != "真实笔名" || author.PlatformID != platform.ID {
		t.Fatalf("author update: %+v", author)
	}
	call("GET", fmt.Sprintf("/api/v2/authors?id=%d", author.ID), nil, 200, &author)
	if author.Name == nil || *author.Name != "真实笔名" {
		t.Fatal("author name was not persisted")
	}
	call("GET", fmt.Sprintf("/api/v2/authors?platformId=%d", platform.ID), nil, 200, &authors)
	if len(authors) != 1 || authors[0].Name == nil || *authors[0].Name != "真实笔名" {
		t.Fatalf("author list name missing: %+v", authors)
	}
	call("PUT", fmt.Sprintf("/api/v2/authors?id=%d", author.ID), map[string]any{"name": nil}, 200, &author)
	if author.Name != nil {
		t.Fatal("author name was not cleared")
	}
	call("POST", "/api/v2/authors", storage.AuthorInput{PlatformID: 2147483647}, 400, nil)
	call("GET", "/api/v2/authors?id=2147483647", nil, 404, nil)
	var emptyNovel storage.Novel
	call("POST", "/api/v2/novelinfo", storage.NovelInput{PlatformID: platform.ID, Title: "Empty"}, 201, &emptyNovel)
	var toc storage.TOC
	tocPath := fmt.Sprintf("/api/v2/toc?novelId=%d", emptyNovel.ID)
	call("GET", tocPath, nil, 200, &toc)
	if toc.NovelID != emptyNovel.ID || toc.Volumes == nil || toc.Chapters == nil || len(toc.Volumes) != 0 || len(toc.Chapters) != 0 {
		t.Fatalf("empty TOC: %+v", toc)
	}
	call("PUT", "/api/v2/volumes", storage.VolumeInput{NovelID: emptyNovel.ID, Order: 2, Title: "Empty volume"}, 200, nil)
	call("GET", tocPath, nil, 200, &toc)
	if len(toc.Volumes) != 1 || toc.Volumes[0].Order != 2 || len(toc.Chapters) != 0 {
		t.Fatalf("volumes-only TOC: %+v", toc)
	}
	call("DELETE", fmt.Sprintf("/api/v2/volumes?novelId=%d&order=2", emptyNovel.ID), nil, 204, nil)
	call("PUT", "/api/v2/content", storage.ChapterInput{NovelID: emptyNovel.ID, Order: 5, Title: "Ungrouped", Content: "Body"}, 200, nil)
	call("GET", tocPath, nil, 200, &toc)
	if len(toc.Volumes) != 0 || len(toc.Chapters) != 1 || toc.Chapters[0].Order != 5 || toc.Chapters[0].VolumeID != nil {
		t.Fatalf("chapters-only TOC: %+v", toc)
	}
	call("DELETE", fmt.Sprintf("/api/v2/novelinfo?id=%d", emptyNovel.ID), nil, 204, nil)
	call("GET", tocPath, nil, 404, nil)
	volumeOrder := int32(1)
	input := storage.UploadInput{
		Novel:   &storage.NovelInput{PlatformID: platform.ID, AuthorID: &author.ID, Title: " Book "},
		Volumes: []storage.UploadVolume{{Order: 2, Title: "Part 2"}, {Order: 1, Title: " Part 1 "}},
		Chapters: []storage.UploadChapter{
			{Order: 2, Title: "Chapter 2", Content: "Ungrouped"},
			{Order: 1, VolumeOrder: &volumeOrder, Title: " Chapter 1 ", Content: " body\n"},
		},
	}
	var first storage.UploadResult
	call("POST", "/api/v2/upload", input, 201, &first)
	if first.Novel.Title != "Book" || first.Novel.Status != "ONGOING" || first.ChapterCount != 2 || len(first.Volumes) != 2 || len(first.Chapters) != 2 || first.Chapters[0].VolumeID != nil || first.Chapters[1].VolumeID == nil {
		t.Fatalf("upload: %+v", first)
	}
	novelID := first.Novel.ID
	var novels []storage.Novel
	call("GET", fmt.Sprintf("/api/v2/novelinfo?platformId=%d&authorId=%d", platform.ID, author.ID), nil, 200, &novels)
	if len(novels) != 1 || novels[0].ID != novelID {
		t.Fatalf("uploaded novel missing: %+v", novels)
	}
	var volumes []storage.Volume
	call("GET", fmt.Sprintf("/api/v2/volumes?novelId=%d", novelID), nil, 200, &volumes)
	if len(volumes) != 2 || volumes[0].Order != 1 || volumes[0].Title != "Part 1" {
		t.Fatalf("volumes: %+v", volumes)
	}
	var chapters []storage.Chapter
	call("GET", fmt.Sprintf("/api/v2/content?novelId=%d", novelID), nil, 200, &chapters)
	if len(chapters) != 2 || chapters[0].Order != 1 || chapters[0].Content != " body\n" || chapters[0].VolumeID == nil || *chapters[0].VolumeID != volumes[0].ID {
		t.Fatalf("chapters: %+v", chapters)
	}
	call("GET", fmt.Sprintf("/api/v2/toc?novelId=%d", novelID), nil, 200, &toc)
	if toc.NovelID != novelID || len(toc.Volumes) != 2 || len(toc.Chapters) != 2 || toc.Volumes[0].ID != volumes[0].ID || toc.Volumes[1].ID != volumes[1].ID {
		t.Fatalf("mixed TOC: %+v", toc)
	}
	for i, summary := range toc.Chapters {
		chapter := chapters[i]
		if summary.ID != chapter.ID || summary.NovelID != chapter.NovelID || summary.Order != chapter.Order || summary.Title != chapter.Title || !summary.UpdatedAt.Equal(chapter.UpdatedAt) {
			t.Fatalf("chapter summary mismatch: %+v, chapter: %+v", summary, chapter)
		}
	}
	if toc.Chapters[0].VolumeID == nil || *toc.Chapters[0].VolumeID != volumes[0].ID || toc.Chapters[1].VolumeID != nil {
		t.Fatalf("TOC volume associations: %+v", toc.Chapters)
	}
	var isolatedNovel storage.Novel
	call("POST", "/api/v2/novelinfo", storage.NovelInput{PlatformID: platform.ID, Title: "Other book"}, 201, &isolatedNovel)
	call("PUT", "/api/v2/volumes", storage.VolumeInput{NovelID: isolatedNovel.ID, Order: 1, Title: "Other volume"}, 200, nil)
	call("PUT", "/api/v2/content", storage.ChapterInput{NovelID: isolatedNovel.ID, Order: 1, Title: "Other chapter", Content: "Private body"}, 200, nil)
	call("GET", fmt.Sprintf("/api/v2/toc?novelId=%d", novelID), nil, 200, &toc)
	if len(toc.Volumes) != 2 || len(toc.Chapters) != 2 {
		t.Fatalf("TOC included another novel: %+v", toc)
	}
	for _, chapter := range toc.Chapters {
		if chapter.NovelID != novelID {
			t.Fatalf("TOC included another novel's chapter: %+v", chapter)
		}
	}
	var rawTOC struct {
		Chapters []map[string]json.RawMessage `json:"chapters"`
	}
	call("GET", fmt.Sprintf("/api/v2/toc?novelId=%d", novelID), nil, 200, &rawTOC)
	for _, chapter := range rawTOC.Chapters {
		if _, ok := chapter["content"]; ok {
			t.Fatal("TOC leaked chapter content")
		}
	}
	call("DELETE", fmt.Sprintf("/api/v2/novelinfo?id=%d", isolatedNovel.ID), nil, 204, nil)
	var second storage.UploadResult
	input = storage.UploadInput{NovelID: &novelID,
		Volumes:  []storage.UploadVolume{{Order: 1, Title: "Renamed"}},
		Chapters: []storage.UploadChapter{{Order: 1, VolumeOrder: &volumeOrder, Title: "Chapter 1", Content: "Replacement"}}}
	call("POST", "/api/v2/upload", input, 200, &second)
	call("POST", "/api/v2/upload", input, 200, &second)
	if second.ChapterCount != 1 || second.Chapters[0].ID != chapters[0].ID || second.Volumes[0].ID != volumes[0].ID || !second.Volumes[0].CreatedAt.Equal(volumes[0].CreatedAt) {
		t.Fatalf("retry changed IDs: %+v", second)
	}
	var volume storage.Volume
	call("PUT", "/api/v2/volumes", storage.VolumeInput{NovelID: novelID, Order: 1, Title: " Part 1 "}, 200, &volume)
	if volume.ID != volumes[0].ID || !volume.CreatedAt.Equal(volumes[0].CreatedAt) {
		t.Fatalf("volume upsert: %+v", volume)
	}
	call("GET", fmt.Sprintf("/api/v2/volumes?novelId=%d&order=1", novelID), nil, 200, &volume)
	call("DELETE", fmt.Sprintf("/api/v2/volumes?novelId=%d&order=1", novelID), nil, 409, nil)
	call("DELETE", fmt.Sprintf("/api/v2/volumes?novelId=%d&order=2", novelID), nil, 204, nil)
	call("DELETE", fmt.Sprintf("/api/v2/volumes?novelId=%d&order=2", novelID), nil, 404, nil)
	call("GET", fmt.Sprintf("/api/v2/volumes?novelId=%d&offset=100", novelID), nil, 200, &volumes)
	if len(volumes) != 0 {
		t.Fatalf("empty volume page: %+v", volumes)
	}
	call("GET", "/api/v2/volumes?novelId=2147483647", nil, 404, nil)
	call("PUT", "/api/v2/volumes", storage.VolumeInput{NovelID: 2147483647, Order: 1, Title: "Missing"}, 400, nil)

	// Failure after volume writes must roll back both new novels and existing data.
	missingOrder := int32(99)
	input = storage.UploadInput{NovelID: &novelID, Volumes: []storage.UploadVolume{{Order: 1, Title: "Must roll back"}},
		Chapters: []storage.UploadChapter{{Order: 1, VolumeOrder: &missingOrder, Title: "C", Content: "Must roll back"}}}
	call("POST", "/api/v2/upload", input, 400, nil)
	call("GET", fmt.Sprintf("/api/v2/volumes?novelId=%d&order=1", novelID), nil, 200, &volume)
	if volume.Title != "Part 1" {
		t.Fatal("failed upload changed volume")
	}
	input.NovelID = nil
	input.Novel = &storage.NovelInput{PlatformID: platform.ID, Title: "Must not exist"}
	call("POST", "/api/v2/upload", input, 400, nil)
	call("GET", "/api/v2/novelinfo", nil, 200, &novels)
	if len(novels) != 1 {
		t.Fatalf("failed upload created a novel: %+v", novels)
	}
	// A database error in a later batched chapter must roll back earlier upserts.
	input = storage.UploadInput{NovelID: &novelID, Chapters: []storage.UploadChapter{
		{Order: 1, Title: "C", Content: "Must roll back"},
		{Order: 3, Title: "C", Content: "invalid\x00text"},
	}}
	if _, err := store.Upload(ctx, input); err == nil {
		t.Fatal("expected database text error")
	}
	var chapter storage.Chapter
	call("GET", fmt.Sprintf("/api/v2/content?novelId=%d&order=1", novelID), nil, 200, &chapter)
	if chapter.Content != "Replacement" || chapter.VolumeID == nil {
		t.Fatalf("failed batch changed chapter: %+v", chapter)
	}

	// Exercise the maximum supported chapter count through the authenticated route.
	bulk := storage.UploadInput{NovelID: &novelID, Volumes: make([]storage.UploadVolume, MaxUploadVolumes), Chapters: make([]storage.UploadChapter, MaxUploadChapters)}
	for i := range bulk.Volumes {
		bulk.Volumes[i] = storage.UploadVolume{Order: int32(i + 1), Title: "V"}
	}
	for i := range bulk.Chapters {
		bulk.Chapters[i] = storage.UploadChapter{Order: int32(i + 10), Title: "C", Content: "B"}
	}
	call("POST", "/api/v2/upload", bulk, 200, &second)
	if second.ChapterCount != MaxUploadChapters || len(second.Chapters) != MaxUploadChapters || len(second.Volumes) != MaxUploadVolumes {
		t.Fatal("maximum batch incomplete")
	}
	call("GET", fmt.Sprintf("/api/v2/toc?novelId=%d", novelID), nil, 200, &toc)
	if len(toc.Volumes) != MaxUploadVolumes || len(toc.Chapters) != MaxUploadChapters+2 {
		t.Fatalf("TOC must not paginate: %d volumes, %d chapters", len(toc.Volumes), len(toc.Chapters))
	}
	for i := 1; i < len(toc.Chapters); i++ {
		if toc.Chapters[i-1].Order >= toc.Chapters[i].Order {
			t.Fatal("TOC chapters are not globally ordered")
		}
	}
	missingID := int32(2147483647)
	bulk.NovelID = &missingID
	call("POST", "/api/v2/upload", bulk, http.StatusNotFound, nil)
	call("DELETE", fmt.Sprintf("/api/v2/novelinfo?id=%d", novelID), nil, 204, nil)
	testReviewReceipts(t, pool)
}
