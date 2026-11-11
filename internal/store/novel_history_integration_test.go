package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestNovelHistoryIntegration(t *testing.T) {
	st := athubTestStore(t)
	ctx := context.Background()
	migration, err := os.ReadFile("../../prisma/migrations/20261110050000_novel_history/migration.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, string(migration), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckNovelHistorySchema(ctx); err != nil {
		t.Fatal(err)
	}
	creator, editor := uuid.New(), uuid.New()
	if _, err := st.pool.Exec(ctx, `INSERT INTO users(id) VALUES($1),($2)`, creator, editor); err != nil {
		t.Fatal(err)
	}
	makeRevision := func(user uuid.UUID, action string) NovelRevision {
		actor, _ := json.Marshal(map[string]string{"id": user.String(), "name": "贡献者", "avatar": ""})
		changes, _ := json.Marshal([]map[string]any{{"entity": "Novel", "action": action, "before": nil, "after": map[string]string{"title": "小说"}}})
		return NovelRevision{ID: uuid.New(), NovelID: 2, UserID: &user, Contributor: actor, Changes: changes}
	}
	batch := uuid.New()
	if err := st.PrepareNovelRevisions(ctx, batch, []NovelRevision{makeRevision(creator, "INSERT")}); err != nil {
		t.Fatal(err)
	}
	if history, err := st.NovelHistory(ctx, 2, 20, 0); err != nil || history.Total != 0 {
		t.Fatal("pending revision visible", err)
	}
	if err := st.FinishNovelRevisions(ctx, batch, true); err != nil {
		t.Fatal(err)
	}
	batch = uuid.New()
	if err := st.PrepareNovelRevisions(ctx, batch, []NovelRevision{makeRevision(editor, "UPDATE")}); err != nil {
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
	if err := st.PrepareNovelRevisions(ctx, batch, []NovelRevision{makeRevision(editor, "UPDATE")}); err != nil {
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
	if err := st.PrepareNovelRevisions(ctx, batch, []NovelRevision{revision}); err != nil {
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
}
