package mail

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

// Only these static Outlook wrappers bypass escaping. Template data never does.
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
	Year                                                             int
}

func renderTemplate(data templateData) (string, error) {
	data.Year = time.Now().Year()
	var output bytes.Buffer
	if err := messageTemplate.Execute(&output, data); err != nil {
		return "", err
	}
	return output.String(), nil
}

func RenderVerification(code, purpose string) (string, error) {
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return "", errors.New("invalid verification code")
	}
	purposeText := "你正在验证或绑定 ALBATEXTUS 账户邮箱"
	if purpose == "register" {
		purposeText = "你正在注册 ALBATEXTUS 账户"
	}
	return renderTemplate(templateData{Subject: "ALBATEXTUS 邮箱验证码", Preview: "请验证你的邮箱，验证码 5 分钟内有效", BrandNote: "账户服务", Eyebrow: "VERIFY YOUR EMAIL", Heading: "验证你的邮箱", PurposeText: purposeText, Code: code})
}
