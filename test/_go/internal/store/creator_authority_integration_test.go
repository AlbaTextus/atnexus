package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// editAccess reports whether an account is admitted to what editors of a novel
// are shown. A refusal must come before the read, and an admission must run it
// exactly once.
func editAccess(t *testing.T, p *Postgres, user uuid.UUID) bool {
	t.Helper()
	reads := 0
	err := p.WithCreatorEditAccess(context.Background(), user, func() error {
		reads++
		return nil
	})
	switch {
	case err == nil && reads == 1:
		return true
	case errors.Is(err, ErrATHubPermissionDenied) && reads == 0:
		return false
	}
	t.Fatalf("edit access ended with err=%v after %d reads", err, reads)
	return false
}

func setEditRule(t *testing.T, p *Postgres, role string, edit ATHubEndpoint, allowed bool) {
	t.Helper()
	if _, err := p.pool.Exec(context.Background(), `INSERT INTO athub_access_rules(role,operation,method,path,allowed) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(role,operation,method,path) DO UPDATE SET allowed=EXCLUDED.allowed`, role, edit.Operation, edit.Method, edit.Path, allowed); err != nil {
		t.Fatal(err)
	}
}

// An account is shown what others have submitted for a novel when its level
// may change an existing novel in at least one way, and its own access to the
// content library has not been taken away.
func TestCreatorEditAccessFollowsTheNovelEditRules(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	users := map[string]uuid.UUID{}
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		users[role], _ = policyUser(t, p, role)
		// As migrated, a level that may upload or edit may edit novels.
		if got, want := editAccess(t, p, users[role]), RoleLevel(role) >= 2; got != want {
			t.Fatalf("role=%s by default: access=%v, want %v", role, got, want)
		}
	}
	// One way to change a novel is enough, whichever it is.
	for _, edit := range ATHubNovelEdits() {
		setEditRule(t, p, "USER", edit, true)
		if !editAccess(t, p, users["USER"]) {
			t.Fatalf("%s %s alone does not admit", edit.Method, edit.Path)
		}
		setEditRule(t, p, "USER", edit, false)
		if editAccess(t, p, users["USER"]) {
			t.Fatalf("%s %s admits after it was turned off", edit.Method, edit.Path)
		}
	}
	// A level keeps its access until the last of them is turned off. Uploading
	// new novels is then still allowed, and is no way to edit one.
	edits := ATHubNovelEdits()
	for i, edit := range edits {
		if !editAccess(t, p, users["SUPER_ADMIN"]) {
			t.Fatalf("access was lost with %d of %d ways to edit left", len(edits)-i, len(edits))
		}
		setEditRule(t, p, "SUPER_ADMIN", edit, false)
	}
	if editAccess(t, p, users["SUPER_ADMIN"]) {
		t.Fatal("an account with no way to edit a novel was admitted")
	}
	policy, err := p.ATHubPolicy(ctx, "SUPER_ADMIN")
	if err != nil || !policy.Rule("upload", "POST", "/api/v2/upload").Allowed {
		t.Fatalf("the batch upload was meant to stay allowed: %+v err=%v", policy, err)
	}
	// The other levels have their own rules.
	if !editAccess(t, p, users["LEVEL_2"]) || !editAccess(t, p, users["ADMIN"]) {
		t.Fatal("the rules of one level decided for another")
	}
}

func TestCreatorEditAccessIsRefusedToAccountsWithoutTheLibrary(t *testing.T) {
	p := athubTestStore(t)
	ctx := context.Background()
	for _, scenario := range []struct{ name, restrict, restore string }{
		{"barred from the content library", `UPDATE users SET athub_disabled=true WHERE id=$1`, `UPDATE users SET athub_disabled=false WHERE id=$1`},
		{"banned", `UPDATE users SET is_active=false WHERE id=$1`, `UPDATE users SET is_active=true WHERE id=$1`},
	} {
		user, _ := policyUser(t, p, "ADMIN")
		if !editAccess(t, p, user) {
			t.Fatalf("%s: an unrestricted administrator was refused", scenario.name)
		}
		if _, err := p.pool.Exec(ctx, scenario.restrict, user); err != nil {
			t.Fatal(err)
		}
		if editAccess(t, p, user) {
			t.Fatalf("%s: the account was admitted", scenario.name)
		}
		if _, err := p.pool.Exec(ctx, scenario.restore, user); err != nil {
			t.Fatal(err)
		}
		if !editAccess(t, p, user) {
			t.Fatalf("%s: the account stayed refused after the restriction was lifted", scenario.name)
		}
	}
	// An account that does not exist is an error, not an admission.
	reads := 0
	if err := p.WithCreatorEditAccess(ctx, uuid.New(), func() error {
		reads++
		return nil
	}); err == nil || reads != 0 {
		t.Fatalf("an unknown account: err=%v reads=%d", err, reads)
	}
}

// What the read fails with is what the caller gets.
func TestCreatorEditAccessReturnsTheErrorOfTheRead(t *testing.T) {
	p := athubTestStore(t)
	user, _ := policyUser(t, p, "LEVEL_2")
	failure := errors.New("creator database unavailable")
	if err := p.WithCreatorEditAccess(context.Background(), user, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("err=%v, want the read's own error", err)
	}
	// The failed read left nothing behind that keeps the next one out.
	if !editAccess(t, p, user) {
		t.Fatal("a failed read left the account refused")
	}
}
