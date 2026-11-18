package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/AlbaTextus/atnexus/internal/store"
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
		{"LEVEL_2", 2, false},
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

func TestLevelTwoApplicationMinimum(t *testing.T) {
	w := httptest.NewRecorder()
	app, ok := readApplication(w, keyRequest("POST", `{
		"name":"LT2 app","homepage_url":"https://example.test","minimum_role":"LEVEL_2",
		"redirect_uris":["https://example.test/callback"]
	}`))
	if !ok || app.MinimumRole != "LEVEL_2" {
		t.Fatalf("LT2 application minimum rejected: status=%d body=%s", w.Code, w.Body)
	}
}
