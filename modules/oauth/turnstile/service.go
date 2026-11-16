package turnstile

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlbaTextus/atnexus/internal/config"
)

const Action = "authenticate"
const verifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

var (
	ErrRequired    = errors.New("turnstile required")
	ErrRejected    = errors.New("turnstile rejected")
	ErrUnavailable = errors.New("turnstile unavailable")
)

type Service struct {
	cfg    config.Turnstile
	client *http.Client
}

func New(cfg config.Config) (*Service, error) {
	if err := cfg.Turnstile.Validate(cfg.Env); err != nil {
		return nil, err
	}
	return &Service{cfg: cfg.Turnstile, client: &http.Client{
		Timeout:       8 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (s *Service) Verify(ctx context.Context, token string) error {
	if !s.cfg.Enabled() {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrRequired
	}
	if len(token) > 2048 {
		return ErrRejected
	}
	form := url.Values{"secret": {s.cfg.SecretKey}, "response": {token}}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrUnavailable
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(r)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	var result struct {
		Success     bool      `json:"success"`
		Hostname    string    `json:"hostname"`
		Action      string    `json:"action"`
		ChallengeTS time.Time `json:"challenge_ts"`
	}
	// Never log the token, secret, or provider response body.
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<10))
	if err != nil || json.Unmarshal(data, &result) != nil {
		return ErrUnavailable
	}
	if !result.Success {
		return ErrRejected
	}
	// Official test keys return synthetic hostname/action/timestamp metadata.
	// Test mode is explicitly configured and rejected in production by Validate.
	if s.cfg.TestMode {
		return nil
	}
	hostOK := false
	for _, hostname := range s.cfg.AllowedHostnames {
		if strings.EqualFold(result.Hostname, hostname) {
			hostOK = true
			break
		}
	}
	age := time.Since(result.ChallengeTS)
	if !hostOK || result.Action != Action || result.ChallengeTS.IsZero() || age > 5*time.Minute || age < -time.Minute {
		return ErrRejected
	}
	return nil
}
