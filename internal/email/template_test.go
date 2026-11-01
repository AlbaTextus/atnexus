package email

import (
	"strings"
	"testing"
)

func TestVerificationTemplate(t *testing.T) {
	for _, purpose := range []string{"register", "account"} {
		t.Run(purpose, func(t *testing.T) {
			html, err := RenderVerification("012345", purpose)
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{"012345", "VERIFY YOUR EMAIL", "ALBATEXTUS", "<!--[if mso]>"} {
				if !strings.Contains(html, text) {
					t.Errorf("verification template is missing %q", text)
				}
			}
		})
	}
}

func TestVerificationTemplateRejectsInvalidCode(t *testing.T) {
	for _, code := range []string{"", "12345", "1234567", "abcdef", "<code>"} {
		if _, err := RenderVerification(code, "register"); err == nil {
			t.Errorf("invalid code was accepted: %q", code)
		}
	}
}
