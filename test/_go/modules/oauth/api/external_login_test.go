package httpapi

import "testing"

func TestSafeLoginReturn(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"library home", "/library", "/library"},
		{"library filters", "/library?tagId=2&authorId=3", "/library?tagId=2&authorId=3"},
		{"book history", "/library/books/123/history", "/library/books/123/history"},
		{"account security", "/account/security?oauth_linked=1", "/account/security?oauth_linked=1"},
		{"admin page", "/admin/users", "/admin/users"},
		{"OAuth authorize", "/oauth/authorize?client_id=test", "/oauth/authorize?client_id=test"},
		{"no destination", "", "/account"},
		{"different route prefix", "/library-other", "/account"},
		{"outside allowed routes", "/library/../settings", "/account"},
		{"off-site URL", "https://example.invalid/library", "/account"},
		{"host-relative URL", "//example.invalid/library", "/account"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeLoginReturn(tc.input); got != tc.want {
				t.Errorf("safeLoginReturn(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
