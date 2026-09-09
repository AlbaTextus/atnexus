package httpapi

import (
	"testing"

	"github.com/AlbaTextus/connect/internal/store"
)

func TestUserLevelContract(t *testing.T) {
	for _, tc := range []struct {
		role  string
		level int
		admin bool
	}{
		{"USER", 1, false},
		{"ADMIN", 3, true},
		{"SUPER_ADMIN", 4, true},
		{"LEVEL_2", 0, false},
		{"2", 0, false},
		{"", 0, false},
	} {
		t.Run(tc.role, func(t *testing.T) {
			user := store.User{Role: tc.role}
			view := userView(user)
			if view["level"] != tc.level || view["role"] != tc.role {
				t.Fatalf("unexpected role/level: %v/%v", view["role"], view["level"])
			}
			if isAdmin(user) != tc.admin || validUserRole(tc.role) != (tc.level > 0) {
				t.Fatal("role assignment or administrator privileges do not match the level")
			}
		})
	}
}
