package store

import (
	"encoding/json"
	"testing"
)

func TestATHubPolicyRejectsPrivilegeEscalation(t *testing.T) {
	for _, role := range []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"} {
		policy := ATHubRolePolicy{Role: role}
		for _, operation := range []string{"read", "upload", "edit", "review", "delete"} {
			policy.Rules = append(policy.Rules, ATHubAccessRule{Operation: operation, Allowed: operation == "read"})
		}
		if !validATHubPolicy(policy) {
			t.Fatal("valid complete policy rejected")
		}
		for i := range policy.Rules {
			next := policy
			next.Rules = append([]ATHubAccessRule(nil), policy.Rules...)
			next.Rules[i].Allowed = true
			want := !(next.Rules[i].Operation == "review" && RoleLevel(role) < 3) && !(next.Rules[i].Operation == "delete" && RoleLevel(role) < 4)
			if validATHubPolicy(next) != want {
				t.Fatalf("role=%s operation=%s privilege escalation", role, next.Rules[i].Operation)
			}
		}
		duplicate := policy
		duplicate.Rules = append(duplicate.Rules, policy.Rules[0])
		if validATHubPolicy(duplicate) {
			t.Fatal("duplicate rules accepted")
		}
	}
}
func TestATHubUploadCannotBypassEditPermission(t *testing.T) {
	edits := ATHubUploadEdits("/api/v2/upload", json.RawMessage(`{"novelId":1,"chapters":[{"order":1}],"volumes":[{"order":1}]}`))
	if len(edits) != 2 || edits[0].Path != "/api/v2/content" || edits[1].Path != "/api/v2/volumes" {
		t.Fatalf("missing upsert checks: %+v", edits)
	}
	if len(ATHubUploadEdits("/api/v2/upload", json.RawMessage(`{"novel":{"title":"新作品"},"chapters":[{}]}`))) != 0 {
		t.Fatal("new novel upload incorrectly requires existing content")
	}
}
