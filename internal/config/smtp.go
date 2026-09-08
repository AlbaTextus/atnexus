package config

import (
	"errors"
	"net/mail"
	"os"
	"strconv"
	"strings"
)

type SMTP struct {
	Host, User, Pass, From string
	Port                   int
	Secure                 bool
}

func (s SMTP) Enabled() bool { return s.Host != "" && s.User != "" && s.Pass != "" }

func loadSMTP() (SMTP, error) {
	s := SMTP{Host: strings.TrimSpace(os.Getenv("SMTP_HOST")), User: strings.TrimSpace(os.Getenv("SMTP_USER")), Pass: os.Getenv("SMTP_PASS"), From: strings.TrimSpace(os.Getenv("SMTP_FROM"))}
	var err error
	s.Port, err = strconv.Atoi(getenv("SMTP_PORT", "587"))
	if err != nil || s.Port < 1 || s.Port > 65535 {
		return s, errors.New("SMTP_PORT must be between 1 and 65535")
	}
	s.Secure, err = getBool("SMTP_SECURE", s.Port == 465)
	if err != nil {
		return s, err
	}
	if s.Host == "" && s.User == "" && s.Pass == "" && s.From == "" {
		return s, nil
	}
	if !s.Enabled() {
		return s, errors.New("SMTP_HOST, SMTP_USER and SMTP_PASS must be configured together")
	}
	if strings.ContainsAny(s.Host, "/\r\n\t ") {
		return s, errors.New("SMTP_HOST must be a hostname")
	}
	if s.From == "" {
		s.From = s.User
	}
	if _, err := mail.ParseAddress(s.From); err != nil || strings.ContainsAny(s.From, "\r\n") {
		return s, errors.New("SMTP_FROM (or SMTP_USER) must be a valid sender address")
	}
	return s, nil
}
