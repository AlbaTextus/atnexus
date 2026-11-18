package store

import "testing"

func TestUserManagementRoleHierarchy(t *testing.T) {
	roles := []string{"USER", "LEVEL_2", "ADMIN", "SUPER_ADMIN"}
	for _, actor := range roles {
		for _, target := range roles {
			want := actor == "SUPER_ADMIN" || (actor == "ADMIN" && (target == "USER" || target == "LEVEL_2"))
			if canManageUser(User{Role: actor}, target) != want {
				t.Errorf("actor=%s target=%s want=%v", actor, target, want)
			}
		}
	}
}
