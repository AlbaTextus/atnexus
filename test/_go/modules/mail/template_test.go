package mail

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
			for _, text := range []string{"012345", "VERIFY YOUR EMAIL", "ALBATEXTUS", "<!--[if mso]>", "text-align:center;", "© "} {
				if !strings.Contains(html, text) {
					t.Errorf("verification template is missing %q", text)
				}
			}
			for _, removed := range []string{"返回刚才的页面", "每一次连接，都有归处", "此邮件由系统自动发送"} {
				if strings.Contains(html, removed) {
					t.Errorf("verification template still contains %q", removed)
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
