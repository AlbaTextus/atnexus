package email

import (
	"bytes"
	_ "embed"
	"errors"
	"html/template"
	"strings"
	"time"
)

//go:embed templates/message.html
var messageHTML string

// Only these static Outlook wrappers bypass escaping. Message content never does.
var messageTemplate = template.Must(template.New("message").Funcs(template.FuncMap{
	"outlookSettings": func() template.HTML {
		return template.HTML(`<!--[if mso]><noscript><xml><o:OfficeDocumentSettings><o:PixelsPerInch>96</o:PixelsPerInch></o:OfficeDocumentSettings></xml></noscript><![endif]-->`)
	},
	"outlookStart": func() template.HTML {
		return template.HTML(`<!--[if mso]><table role="presentation" width="560" align="center" cellpadding="0" cellspacing="0" border="0"><tr><td><![endif]-->`)
	},
	"outlookEnd": func() template.HTML { return template.HTML(`<!--[if mso]></td></tr></table><![endif]-->`) },
}).Parse(messageHTML))

type templateData struct {
	Subject, Preview, BrandNote, Eyebrow, Heading, PurposeText, Code string
	FooterText                                                       string
	Lines                                                            []string
	Year                                                             int
}

func renderTemplate(data templateData) (string, error) {
	data.Year = time.Now().Year()
	if data.FooterText == "" {
		data.FooterText = "此邮件由系统自动发送，请勿直接回复。"
	}
	var output bytes.Buffer
	if err := messageTemplate.Execute(&output, data); err != nil {
		return "", err
	}
	return output.String(), nil
}

// RenderMessage is shared by the SMTP sender and the administrator's preview.
func RenderMessage(subject, content string) (string, error) {
	return renderTemplate(templateData{Subject: subject, Preview: "你收到一条来自 ALBATEXTUS 的社区消息。", BrandNote: "社区消息", Eyebrow: "A MESSAGE FOR YOU", Heading: subject, Lines: strings.Split(strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n"), "\n")})
}

// RenderAnnouncement uses the same brand layout with wording for an on-site notice.
func RenderAnnouncement(subject, content string) (string, error) {
	return renderTemplate(templateData{Subject: subject, Preview: "ALBATEXTUS 社区公告", BrandNote: "社区公告", Eyebrow: "COMMUNITY UPDATE", Heading: subject, FooterText: "此公告由 ALBATEXTUS 发布。", Lines: strings.Split(strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n"), "\n")})
}

func RenderVerification(code, purpose string) (string, error) {
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return "", errors.New("invalid verification code")
	}
	purposeText := "你正在验证或绑定 ALBATEXTUS 账户邮箱。"
	if purpose == "register" {
		purposeText = "你正在注册 ALBATEXTUS 账户。"
	}
	return renderTemplate(templateData{Subject: "ALBATEXTUS 邮箱验证码", Preview: "请验证你的邮箱，验证码 5 分钟内有效。", BrandNote: "账户服务", Eyebrow: "VERIFY YOUR EMAIL", Heading: "验证你的邮箱", PurposeText: purposeText, Code: code})
}
