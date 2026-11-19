package storage

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// editGate is an authority that remembers who asked to read what editors are
// shown, and admits them unless it holds a refusal.
type editGate struct {
	openAuthority
	asked   []uuid.UUID
	refusal error
}

func (g *editGate) WithCreatorEditAccess(_ context.Context, actor uuid.UUID, read func() error) error {
	g.asked = append(g.asked, actor)
	if g.refusal != nil {
		return g.refusal
	}
	return read()
}

// queueAt stores a review as it would stand at the given time, with the given
// status and baseline. An empty baseline is stored as none.
func queueAt(t *testing.T, pool *pgxpool.Pool, status, baseline string, at time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var stored *string
	if baseline != "" {
		stored = &baseline
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO athub_reviews(id,user_id,contributor,operation,method,path,query,payload,status,baseline,created_at)
		VALUES($1,$2,'{"name":"投稿者"}','edit','PUT','/api/v2/novelinfo','id=17','{"title":"新标题"}',$3,$4::jsonb,$5)`, id, uuid.New(), status, stored, at); err != nil {
		t.Fatal(err)
	}
	return id
}

func pendingIDs(list store.ATHubPendingChangeList) []uuid.UUID {
	ids := make([]uuid.UUID, len(list.Items))
	for i, item := range list.Items {
		ids[i] = item.ID
	}
	return ids
}

// The changes shown to someone editing a novel are the reviews still awaiting a
// decision whose baseline lists that novel, in the order they were submitted.
// Each comes with the baseline cut down to the novel that was asked for.
func TestPendingChangesOfANovel(t *testing.T) {
	creator, pool := creatorOnly(t)
	ctx := context.Background()
	gate := &editGate{}
	creator.authority = gate
	actor := uuid.New()
	start := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	const title = `{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"旧标题"},"after":{"title":"新标题"}}`
	const cover = `{"key":"Novel","action":"UPDATE","id":17,"before":{"coverUrl":null},"after":{"coverUrl":"https://img.example.test/c.webp"}}`
	const other = `{"key":"Novel","action":"UPDATE","id":18,"before":{"title":"另一部作品的旧标题"},"after":{"title":"另一部作品的新标题"}}`

	third := queueAt(t, pool, "pending", `{"v":1,"novels":{"17":[`+cover+`]}}`, start.Add(3*time.Hour))
	first := queueAt(t, pool, "pending", `{"v":1,"novels":{"17":[`+title+`]},"shared":[{"key":"Tag:3","action":"UPDATE","id":3,"before":{"name":"旧标签"}}]}`, start.Add(time.Hour))
	// One change can alter several novels; it is listed for each of them.
	second := queueAt(t, pool, "pending", `{"v":1,"novels":{"18":[`+other+`],"17":[`+title+`,`+cover+`]}}`, start.Add(2*time.Hour))
	elsewhere := queueAt(t, pool, "pending", `{"v":1,"novels":{"18":[`+other+`]}}`, start)
	// None of these is a change that awaits a decision on novel 17.
	queueAt(t, pool, "pending", "", start)
	queueAt(t, pool, "pending", `{"v":1,"shared":[{"key":"Tag:17","action":"UPDATE","id":17,"before":{"name":"旧标签"}}]}`, start)
	near := queueAt(t, pool, "pending", `{"v":1,"novels":{"170":[`+title+`],"1":[`+title+`],"7":[`+title+`],"-17":[`+title+`]}}`, start)
	for _, decided := range []string{"approved", "rejected", "failed"} {
		queueAt(t, pool, decided, `{"v":1,"novels":{"17":[`+title+`]}}`, start)
	}

	list, err := creator.PendingATHubChanges(ctx, actor, 17, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []uuid.UUID{first, second, third}; list.Total != 3 || !reflect.DeepEqual(pendingIDs(list), want) {
		t.Fatalf("novel 17: total=%d ids=%v, want %v", list.Total, pendingIDs(list), want)
	}
	if len(gate.asked) != 1 || gate.asked[0] != actor {
		t.Fatalf("the authority was asked about %v, want %s", gate.asked, actor)
	}
	for i, want := range []struct {
		at       time.Time
		baseline string
	}{
		{start.Add(time.Hour), `{"v":1,"novels":{"17":[` + title + `]}}`},
		{start.Add(2 * time.Hour), `{"v":1,"novels":{"17":[` + title + `,` + cover + `]}}`},
		{start.Add(3 * time.Hour), `{"v":1,"novels":{"17":[` + cover + `]}}`},
	} {
		item := list.Items[i]
		if !item.CreatedAt.Equal(want.at) {
			t.Fatalf("item %d was submitted at %s, want %s", i, item.CreatedAt, want.at)
		}
		if !reflect.DeepEqual(decoded(t, item.Baseline), decoded(t, []byte(want.baseline))) {
			t.Fatalf("item %d came with %s, want %s", i, item.Baseline, want.baseline)
		}
	}
	list, err = creator.PendingATHubChanges(ctx, actor, 18, 0)
	if want := []uuid.UUID{elsewhere, second}; err != nil || list.Total != 2 || !reflect.DeepEqual(pendingIDs(list), want) {
		t.Fatalf("novel 18: total=%d ids=%v, want %v, err=%v", list.Total, pendingIDs(list), want, err)
	}
	if want := `{"v":1,"novels":{"18":[` + other + `]}}`; !reflect.DeepEqual(decoded(t, list.Items[1].Baseline), decoded(t, []byte(want))) {
		t.Fatalf("novel 18 came with %s, want %s", list.Items[1].Baseline, want)
	}
	// A novel is told apart by its whole ID: the change queued for novels 1, 7
	// and 170 is listed for each of them and was not listed for novel 17.
	for _, listed := range []int32{1, 7, 170} {
		list, err = creator.PendingATHubChanges(ctx, actor, listed, 0)
		if want := []uuid.UUID{near}; err != nil || list.Total != 1 || !reflect.DeepEqual(pendingIDs(list), want) {
			t.Fatalf("novel %d: total=%d ids=%v, want %v, err=%v", listed, list.Total, pendingIDs(list), want, err)
		}
	}
	// A novel nothing is queued for has an empty list, which is not a nil one:
	// it is served as [].
	for _, untouched := range []int32{71, 117, 1717, 999} {
		list, err = creator.PendingATHubChanges(ctx, actor, untouched, 0)
		if err != nil || list.Total != 0 || list.Items == nil || len(list.Items) != 0 {
			t.Fatalf("novel %d: list=%+v err=%v", untouched, list, err)
		}
	}
}

// Changes submitted at the same instant keep one order from read to read, and
// a novel with more changes than one read returns is read in pages that
// neither repeat nor skip any.
func TestPendingChangesAreReadInPages(t *testing.T) {
	creator, pool := creatorOnly(t)
	ctx := context.Background()
	start := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	const total = pendingChangesPage + 7
	queued := make([]uuid.UUID, total)
	for i := range queued {
		// Changes are submitted three to an instant.
		at := start.Add(time.Duration(i-i%3) * time.Minute)
		queued[i] = queueAt(t, pool, "pending", `{"v":1,"novels":{"40":[{"key":"Novel","action":"UPDATE","id":40,"before":{"title":"旧"},"after":{"title":"新"}}]}}`, at)
	}
	firstPage, err := creator.PendingATHubChanges(ctx, uuid.New(), 40, 0)
	if err != nil || firstPage.Total != total || len(firstPage.Items) != pendingChangesPage {
		t.Fatalf("first page: total=%d items=%d err=%v", firstPage.Total, len(firstPage.Items), err)
	}
	rest, err := creator.PendingATHubChanges(ctx, uuid.New(), 40, pendingChangesPage)
	if err != nil || rest.Total != total || len(rest.Items) != total-pendingChangesPage {
		t.Fatalf("second page: total=%d items=%d err=%v", rest.Total, len(rest.Items), err)
	}
	beyond, err := creator.PendingATHubChanges(ctx, uuid.New(), 40, total)
	if err != nil || beyond.Total != total || beyond.Items == nil || len(beyond.Items) != 0 {
		t.Fatalf("beyond the end: %+v err=%v", beyond, err)
	}
	seen := make(map[uuid.UUID]bool, total)
	all := append(append([]store.ATHubPendingChange{}, firstPage.Items...), rest.Items...)
	for i, item := range all {
		if seen[item.ID] {
			t.Fatalf("change %s was read twice", item.ID)
		}
		seen[item.ID] = true
		if i == 0 {
			continue
		}
		previous := all[i-1]
		if item.CreatedAt.Before(previous.CreatedAt) || (item.CreatedAt.Equal(previous.CreatedAt) && item.ID.String() <= previous.ID.String()) {
			t.Fatalf("change %d (%s at %s) follows %s at %s", i, item.ID, item.CreatedAt, previous.ID, previous.CreatedAt)
		}
	}
	for _, id := range queued {
		if !seen[id] {
			t.Fatalf("change %s was never read", id)
		}
	}
}

// Whoever may not edit novels is told nothing, whatever is queued.
func TestPendingChangesAreKeptFromAccountsThatMayNotEdit(t *testing.T) {
	creator, pool := creatorOnly(t)
	ctx := context.Background()
	queueAt(t, pool, "pending", `{"v":1,"novels":{"17":[{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"旧"},"after":{"title":"新"}}]}}`, time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC))
	for _, refusal := range []error{store.ErrATHubPermissionDenied, store.ErrUserForbidden, errors.New("account database unavailable")} {
		gate := &editGate{refusal: refusal}
		creator.authority = gate
		actor := uuid.New()
		list, err := creator.PendingATHubChanges(ctx, actor, 17, 0)
		if !errors.Is(err, refusal) || list.Total != 0 || len(list.Items) != 0 {
			t.Fatalf("refusal %v: list=%+v err=%v", refusal, list, err)
		}
		if len(gate.asked) != 1 || gate.asked[0] != actor {
			t.Fatalf("the authority was asked about %v, want %s", gate.asked, actor)
		}
	}
}

// With the account database as the authority, the changes are read by an
// account whose level may edit novels, and by no other.
func TestPendingChangesFollowTheAccountsPermissions(t *testing.T) {
	accounts, creator, source, target, _, _ := splitFixture(t)
	ctx := context.Background()
	queued := queueAt(t, target, "pending", `{"v":1,"novels":{"17":[{"key":"Novel","action":"UPDATE","id":17,"before":{"title":"旧"},"after":{"title":"新"}}]}}`, time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC))
	reader, err := accounts.UpsertUser(ctx, store.UserSeed{Email: "pending-reader@example.test", PasswordHash: "unused", Role: "USER"})
	if err != nil {
		t.Fatal(err)
	}
	editor, err := accounts.UpsertUser(ctx, store.UserSeed{Email: "pending-editor@example.test", PasswordHash: "unused", Role: "LEVEL_2"})
	if err != nil {
		t.Fatal(err)
	}
	if list, err := creator.PendingATHubChanges(ctx, reader.ID, 17, 0); !errors.Is(err, store.ErrATHubPermissionDenied) || len(list.Items) != 0 {
		t.Fatalf("an account that may only read: list=%+v err=%v", list, err)
	}
	list, err := creator.PendingATHubChanges(ctx, editor.ID, 17, 0)
	if err != nil || list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != queued {
		t.Fatalf("an account that may edit: list=%+v err=%v", list, err)
	}
	var baseline struct {
		Novels map[string]json.RawMessage `json:"novels"`
	}
	if err := json.Unmarshal(list.Items[0].Baseline, &baseline); err != nil || len(baseline.Novels) != 1 || baseline.Novels["17"] == nil {
		t.Fatalf("baseline=%s err=%v", list.Items[0].Baseline, err)
	}
	if _, err := source.Exec(ctx, `UPDATE users SET athub_disabled=true WHERE id=$1`, editor.ID); err != nil {
		t.Fatal(err)
	}
	if list, err := creator.PendingATHubChanges(ctx, editor.ID, 17, 0); !errors.Is(err, store.ErrATHubPermissionDenied) || len(list.Items) != 0 {
		t.Fatalf("an account barred from the content library: list=%+v err=%v", list, err)
	}
	if list, err := creator.PendingATHubChanges(ctx, uuid.New(), 17, 0); err == nil || len(list.Items) != 0 {
		t.Fatalf("an account that does not exist: list=%+v err=%v", list, err)
	}
}
