package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/AlbaTextus/oauth/internal/config"
)

type Sender interface {
	SendCode(context.Context, string, string, string) error
}

type SMTP struct {
	cfg       config.SMTP
	tlsConfig *tls.Config
}

func New(cfg config.SMTP) *SMTP {
	return &SMTP{cfg: cfg, tlsConfig: &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}}
}

// Never propagate SMTP replies: some servers echo addresses or credentials.
func (s *SMTP) SendCode(ctx context.Context, to, code, purpose string) error {
	label := "验证或绑定邮箱"
	if purpose == "register" {
		label = "注册账户"
	}
	content := fmt.Sprintf("你的 ALBATEXTUS 邮箱验证码是：%s\r\n\r\n用于%s，5 分钟内有效。请勿将验证码告诉他人。\r\n如果不是你本人操作，请忽略此邮件。\r\n", code, label)
	html, err := RenderVerification(code, purpose)
	if err != nil {
		return err
	}
	return s.deliver(ctx, to, "ALBATEXTUS 邮箱验证码", content, html)
}

func (s *SMTP) deliver(ctx context.Context, to, subject, content, html string) error {
	if strings.TrimSpace(subject) == "" || strings.ContainsAny(subject, "\r\n\x00") {
		return errors.New("invalid subject")
	}
	if err := s.send(ctx, to, subject, content, html); err != nil {
		return errors.New("SMTP delivery failed")
	}
	return nil
}

func (s *SMTP) send(ctx context.Context, to, subject, content, html string) error {
	if !s.cfg.Enabled() {
		return errors.New("SMTP not configured")
	}
	recipient, err := mail.ParseAddress(to)
	if err != nil || recipient.Address != to || strings.ContainsAny(to, "\r\n") {
		return errors.New("invalid recipient")
	}
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	var transport net.Conn = conn
	if s.cfg.Secure {
		tlsConn := tls.Client(conn, s.tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		transport = tlsConn
	}
	client, err := smtp.NewClient(transport, s.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if !s.cfg.Secure {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server must support STARTTLS")
		}
		if err := client.StartTLS(s.tlsConfig); err != nil {
			return err
		}
	}
	if err := client.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)); err != nil {
		return err
	}
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message, err := encodeMessage(from.String(), recipient.String(), subject, content, html)
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		return err
	}
	// A successful DATA reply confirms acceptance; a later QUIT failure must not
	// invalidate a code the server already accepted for delivery.
	return writer.Close()
}

func encodeMessage(from, to, subject, plain, html string) ([]byte, error) {
	var body bytes.Buffer
	parts := multipart.NewWriter(&body)
	for _, part := range []struct{ kind, content string }{{"text/plain", plain}, {"text/html", html}} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", part.kind+"; charset=UTF-8")
		header.Set("Content-Transfer-Encoding", "quoted-printable")
		writer, err := parts.CreatePart(header)
		if err != nil {
			return nil, err
		}
		encoder := quotedprintable.NewWriter(writer)
		if _, err := encoder.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := encoder.Close(); err != nil {
			return nil, err
		}
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}
	header := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n", from, to, mime.QEncoding.Encode("UTF-8", subject), time.Now().Format(time.RFC1123Z), parts.Boundary())
	return append([]byte(header), body.Bytes()...), nil
}
