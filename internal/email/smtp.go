package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
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
	if err := s.send(ctx, to, code, purpose); err != nil {
		return errors.New("SMTP delivery failed")
	}
	return nil
}

func (s *SMTP) send(ctx context.Context, to, code, purpose string) error {
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
	label := "验证或绑定邮箱"
	if purpose == "register" {
		label = "注册账户"
	}
	var body bytes.Buffer
	encoder := quotedprintable.NewWriter(&body)
	_, _ = fmt.Fprintf(encoder, "你的 ALBATEXTUS 邮箱验证码是：%s\r\n\r\n用于%s，5 分钟内有效。请勿将验证码告诉他人。\r\n如果不是你本人操作，请忽略此邮件。\r\n", code, label)
	_ = encoder.Close()
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n%s", from.String(), recipient.String(), mime.QEncoding.Encode("UTF-8", "ALBATEXTUS 邮箱验证码"), time.Now().Format(time.RFC1123Z), body.String())
	if _, err := writer.Write([]byte(message)); err != nil {
		return err
	}
	// A successful DATA reply confirms acceptance; a later QUIT failure must not
	// invalidate a code the server already accepted for delivery.
	return writer.Close()
}
