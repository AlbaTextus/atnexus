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
func TestATHubCoverHasItsOwnRuleAndIsNeverReviewed(t *testing.T) {
	if ATHubOperation("POST", ATHubCoverPath) != "upload" || !ATHubKnownPath(ATHubCoverPath) {
		t.Fatal("cover upload is missing from the permission surface")
	}
	for _, method := range []string{"GET", "PUT", "PATCH", "DELETE"} {
		if ATHubOperation(method, ATHubCoverPath) != "" {
			t.Fatalf("%s on the cover path gained an operation", method)
		}
	}
	// It stores a file: nothing to forward for a submitter and nothing to queue.
	if ATHubSubmissionEndpoint("POST", ATHubCoverPath) || ATHubReviewQuery("POST", ATHubCoverPath) != nil {
		t.Fatal("cover upload became a reviewable content submission")
	}
	listed := false
	for _, endpoint := range ATHubEndpoints() {
		listed = listed || endpoint == ATHubEndpoint{"POST", ATHubCoverPath, "upload"}
	}
	if !listed {
		t.Fatal("administrators cannot see the cover rule")
	}
	policy := ATHubRolePolicy{Role: "USER"}
	for _, operation := range []string{"read", "upload", "edit", "review", "delete"} {
		policy.Rules = append(policy.Rules, ATHubAccessRule{Operation: operation, Allowed: operation == "read" || operation == "upload"})
	}
	if !policy.Rule("upload", "POST", ATHubCoverPath).Allowed {
		t.Fatal("cover upload does not follow the level's upload switch by default")
	}
	// Turning batch upload off no longer decides covers, and the reverse.
	policy.Rules = append(policy.Rules,
		ATHubAccessRule{Operation: "upload", Method: "POST", Path: "/api/v2/upload", Allowed: false},
		ATHubAccessRule{Operation: "upload", Method: "POST", Path: ATHubCoverPath, Allowed: true})
	if !validATHubPolicy(policy) {
		t.Fatal("a rule for the cover upload was rejected")
	}
	if !policy.Rule("upload", "POST", ATHubCoverPath).Allowed || policy.Rule("upload", "POST", "/api/v2/upload").Allowed {
		t.Fatal("cover and batch upload rules are not independent")
	}
	policy.Rules[len(policy.Rules)-1].Allowed = false
	if policy.Rule("upload", "POST", ATHubCoverPath).Allowed {
		t.Fatal("the cover rule cannot be turned off")
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
