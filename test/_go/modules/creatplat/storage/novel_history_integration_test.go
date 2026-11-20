package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/AlbaTextus/atnexus/modules/creatplat/repository"
	"github.com/google/uuid"
)

func TestNovelHistoryIntegration(t *testing.T) {
	st, pool := creatorOnly(t)
	ctx := context.Background()
	if err := st.CheckNovelHistorySchema(ctx); err != nil {
		t.Fatal(err)
	}
	// Contributors are accounts of another database; history keeps their IDs.
	creator, editor := uuid.New(), uuid.New()
	makeRevision := func(user uuid.UUID, action string) repository.NovelRevision {
		actor, _ := json.Marshal(map[string]string{"id": user.String(), "name": "贡献者", "avatar": ""})
		changes, _ := json.Marshal([]map[string]any{{"entity": "Novel", "action": action, "before": nil, "after": map[string]string{"title": "小说"}}})
		return repository.NovelRevision{ID: uuid.New(), NovelID: 2, UserID: &user, Contributor: actor, Changes: changes}
	}
	batch := uuid.New()
	if err := st.PrepareNovelRevisions(ctx, batch, []repository.NovelRevision{makeRevision(creator, "INSERT")}); err != nil {
		t.Fatal(err)
	}
	if history, err := st.NovelHistory(ctx, 2, 20, 0); err != nil || history.Total != 0 {
		t.Fatal("pending revision visible", err)
	}
	if err := st.FinishNovelRevisions(ctx, batch, true); err != nil {
		t.Fatal(err)
	}
	batch = uuid.New()
	if err := st.PrepareNovelRevisions(ctx, batch, []repository.NovelRevision{makeRevision(editor, "UPDATE")}); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishNovelRevisions(ctx, batch, true); err != nil {
		t.Fatal(err)
	}
	if history, err := st.NovelHistory(ctx, 2, 1, 1); err != nil || history.Total != 2 || len(history.Revisions) != 1 {
		t.Fatal("pagination", err)
	}
	if history, err := st.NovelHistory(ctx, 2, 1, 100); err != nil || history.Total != 2 || len(history.Revisions) != 0 {
		t.Fatal("empty page lost total", err)
	}
	people, err := st.NovelContributors(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Creator struct {
			ID string `json:"id"`
		} `json:"creator"`
		Contributors []struct {
			ID string `json:"id"`
		} `json:"contributors"`
	}
	if err := json.Unmarshal(people, &data); err != nil || data.Creator.ID != creator.String() || len(data.Contributors) != 1 || data.Contributors[0].ID != editor.String() {
		t.Fatalf("contributors: %s %v", people, err)
	}
	batch = uuid.New()
	if err := st.PrepareNovelRevisions(ctx, batch, []repository.NovelRevision{makeRevision(editor, "UPDATE")}); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishNovelRevisions(ctx, batch, false); err != nil {
		t.Fatal(err)
	}
	if history, err := st.NovelHistory(ctx, 2, 20, 0); err != nil || history.Total != 2 {
		t.Fatal("failed revision visible", err)
	}
	// Legacy entries with only later edits do not acquire a fictitious creator.
	revision := makeRevision(editor, "UPDATE")
	revision.NovelID = 3
	batch = uuid.New()
	if err := st.PrepareNovelRevisions(ctx, batch, []repository.NovelRevision{revision}); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishNovelRevisions(ctx, batch, true); err != nil {
		t.Fatal(err)
	}
	people, err = st.NovelContributors(ctx, 3)
	var legacy struct {
		Creator any `json:"creator"`
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(people, &legacy); err != nil || legacy.Creator != nil {
		t.Fatal("legacy editor became creator")
	}

	t.Run("consecutive submissions are grouped before pagination", func(t *testing.T) {
		start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		var latestID uuid.UUID
		for i := 0; i < 30; i++ {
			user, status := creator, "completed"
			if i == 25 || i == 26 || i == 28 {
				user = editor
			}
			if i == 25 {
				status = "pending"
			}
			if i == 26 {
				status = "failed"
			}
			revision := makeRevision(user, "UPDATE")
			if i == 27 {
				revision.Contributor, _ = json.Marshal(map[string]string{"id": user.String(), "name": "更新后的姓名", "avatar": ""})
				latestID = revision.ID
			}
			change := map[string]any{
				"entity": "Chapter", "action": "UPDATE",
				"before":   map[string]any{"order": i, "title": "原文"},
				"after":    map[string]any{"order": i, "title": "更新"},
				"bodyDiff": []map[string]any{{"beforeLine": 1, "afterLine": 1, "parts": []map[string]string{{"kind": "insert", "text": "新增正文"}}}},
			}
			if i == 0 {
				change = map[string]any{"entity": "Novel", "action": "INSERT", "before": nil, "after": map[string]any{"order": i}}
			}
			changes, _ := json.Marshal([]map[string]any{change})
			if _, err := pool.Exec(ctx, `INSERT INTO novel_revisions
				(id,batch_id,novel_id,user_id,contributor,changes,status,created_at)
				VALUES($1,$2,4,$3,$4,$5,$6,$7)`,
				revision.ID, uuid.New(), user, revision.Contributor, changes, status, start.Add(time.Duration(i)*time.Hour)); err != nil {
				t.Fatal(err)
			}
		}
		// A different novel's editor must not split the import.
		other := makeRevision(editor, "UPDATE")
		if _, err := pool.Exec(ctx, `INSERT INTO novel_revisions
			(id,batch_id,novel_id,user_id,contributor,changes,status,created_at)
			VALUES($1,$2,5,$3,$4,$5,'completed',$6)`,
			other.ID, uuid.New(), editor, other.Contributor, other.Changes, start.Add(10*time.Hour)); err != nil {
			t.Fatal(err)
		}
		for offset, expected := range []int64{1, 1, 26} {
			history, err := st.NovelHistory(ctx, 4, 1, offset)
			if err != nil {
				t.Fatal(err)
			}
			if history.Total != 3 || len(history.Revisions) != 1 || history.Revisions[0].RevisionCount != expected {
				t.Fatalf("offset %d: %+v", offset, history)
			}
			if offset != 2 {
				continue
			}
			revision := history.Revisions[0]
			if revision.ID != latestID || !revision.StartedAt.Equal(start) || !revision.CreatedAt.Equal(start.Add(27*time.Hour)) {
				t.Fatalf("group identity/time range: %+v", revision)
			}
			var actor map[string]string
			if err := json.Unmarshal(revision.Contributor, &actor); err != nil || actor["name"] != "更新后的姓名" {
				t.Fatalf("latest contributor snapshot: %s, %v", revision.Contributor, err)
			}
			type mergedChange struct {
				After    struct{ Order int } `json:"after"`
				BodyDiff json.RawMessage     `json:"bodyDiff"`
			}
			// The list keeps the merged order but leaves the body diffs to the
			// detail page of the same entry.
			var listed []mergedChange
			if err := json.Unmarshal(revision.Changes, &listed); err != nil || len(listed) != 26 || revision.ChangeCount != 26 {
				t.Fatalf("merged changes missing: count=%d %s, %v", revision.ChangeCount, revision.Changes, err)
			}
			page, err := st.NovelRevisionChanges(ctx, 4, revision.ID, 100, 0)
			if err != nil {
				t.Fatal(err)
			}
			var detailed []mergedChange
			if err := json.Unmarshal(page.Changes, &detailed); err != nil || page.Total != 26 || len(detailed) != 26 {
				t.Fatalf("merged change details missing: total=%d %s, %v", page.Total, page.Changes, err)
			}
			for i := range listed {
				order := i
				if i == 25 {
					order = 27
				}
				if listed[i].After.Order != order || len(listed[i].BodyDiff) != 0 {
					t.Fatalf("listed change order lost or body diff kept at %d: %+v", i, listed[i])
				}
				if detailed[i].After.Order != order || (i > 0 && len(detailed[i].BodyDiff) == 0) {
					t.Fatalf("change order or body diff lost at %d: %+v", i, detailed[i])
				}
			}
		}
		history, err := st.NovelHistory(ctx, 4, 1, 3)
		if err != nil || history.Total != 3 || len(history.Revisions) != 0 {
			t.Fatalf("empty grouped page lost total: %+v, %v", history, err)
		}
		var rawCount int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM novel_revisions WHERE novel_id=4`).Scan(&rawCount); err != nil || rawCount != 30 {
			t.Fatal("original audit records changed", rawCount, err)
		}
	})

	t.Run("unknown contributors stay separate with stable timestamp ordering", func(t *testing.T) {
		stamp := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		for _, id := range []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"} {
			if _, err := pool.Exec(ctx, `INSERT INTO novel_revisions
				(id,batch_id,novel_id,contributor,changes,status,created_at)
				VALUES($1,$2,6,'{"name":"未记录账号"}','[]','completed',$3)`, id, uuid.New(), stamp); err != nil {
				t.Fatal(err)
			}
		}
		history, err := st.NovelHistory(ctx, 6, 20, 0)
		if err != nil || history.Total != 2 || len(history.Revisions) != 2 {
			t.Fatalf("unrelated anonymous revisions merged: %+v, %v", history, err)
		}
		if history.Revisions[0].ID.String() != "00000000-0000-0000-0000-000000000002" {
			t.Fatal("equal timestamp ordering is not deterministic")
		}
	})

	t.Run("large entries are listed bounded and read by page", func(t *testing.T) {
		author := uuid.New()
		actor, _ := json.Marshal(map[string]string{"id": author.String(), "name": "作者", "avatar": ""})
		var newest uuid.UUID
		// Two consecutive revisions by one account: 150 chapter edits, then 30 more.
		for round, size := range []int{150, 30} {
			changes := make([]map[string]any, size)
			for i := range changes {
				changes[i] = map[string]any{"entity": "Chapter", "action": "UPDATE", "bodyDiff": []int{round, i},
					"after": map[string]any{"title": "章", "round": round, "index": i}}
			}
			if round == 1 {
				changes[0]["entity"], changes[0]["action"] = "Volume", "INSERT"
			}
			raw, _ := json.Marshal(changes)
			batch, revision := uuid.New(), repository.NovelRevision{ID: uuid.New(), NovelID: 8, UserID: &author, Contributor: actor, Changes: raw}
			if err := st.PrepareNovelRevisions(ctx, batch, []repository.NovelRevision{revision}); err != nil {
				t.Fatal(err)
			}
			if err := st.FinishNovelRevisions(ctx, batch, true); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE novel_revisions SET created_at=$2 WHERE id=$1`, revision.ID,
				time.Date(2026, 11, 2, 0, round, 0, 0, time.UTC)); err != nil {
				t.Fatal(err)
			}
			newest = revision.ID
		}
		history, err := st.NovelHistory(ctx, 8, 20, 0)
		if err != nil || history.Total != 1 || len(history.Revisions) != 1 {
			t.Fatalf("history=%+v err=%v", history, err)
		}
		entry := history.Revisions[0]
		var listed []map[string]any
		var stats []struct {
			Entity, Action string
			Count          int64
		}
		if json.Unmarshal(entry.Changes, &listed) != nil || json.Unmarshal(entry.ChangeStats, &stats) != nil {
			t.Fatal("entry is not JSON")
		}
		if entry.ID != newest || entry.ChangeCount != 180 || len(listed) != 100 {
			t.Fatalf("id=%s count=%d listed=%d", entry.ID, entry.ChangeCount, len(listed))
		}
		for _, change := range listed {
			if _, kept := change["bodyDiff"]; kept {
				t.Fatal("list carries a chapter body diff")
			}
		}
		if len(stats) != 2 || stats[0].Entity != "Chapter" || stats[0].Count != 179 || stats[1].Entity != "Volume" || stats[1].Count != 1 {
			t.Fatalf("stats=%+v", stats)
		}
		page, err := st.NovelRevisionChanges(ctx, 8, newest, 100, 100)
		var rest []map[string]any
		if err != nil || page.Total != 180 || json.Unmarshal(page.Changes, &rest) != nil || len(rest) != 80 {
			t.Fatalf("page total=%d rest=%d err=%v", page.Total, len(rest), err)
		}
		// Oldest first: the page starts inside the first revision and keeps diffs.
		first := rest[0]["after"].(map[string]any)
		if first["round"] != float64(0) || first["index"] != float64(100) || rest[0]["bodyDiff"] == nil || rest[50]["entity"] != "Volume" {
			t.Fatalf("page starts at %+v", rest[0])
		}
		if _, err := st.NovelRevisionChanges(ctx, 8, uuid.New(), 100, 0); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("unknown revision: %v", err)
		}
		if _, err := st.NovelRevisionChanges(ctx, 9, newest, 100, 0); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("revision of another novel: %v", err)
		}
	})
}
