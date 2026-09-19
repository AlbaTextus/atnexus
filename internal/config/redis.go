package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type Redis struct {
	Enabled    bool
	URL        string
	KeyPrefix  string
	SessionTTL time.Duration
}

func loadRedis() (Redis, error) {
	enabled, err := getBool("REDIS_ENABLED", false)
	if err != nil {
		return Redis{}, err
	}
	ttl := 30 * time.Second
	if value := strings.TrimSpace(os.Getenv("REDIS_SESSION_TTL")); value != "" {
		parsedTTL, parseErr := time.ParseDuration(value)
		if parseErr != nil && enabled {
			return Redis{}, fmt.Errorf("REDIS_SESSION_TTL must be a duration: %w", parseErr)
		}
		if parseErr == nil {
			ttl = parsedTTL
		}
	}
	cfg := Redis{
		Enabled:    enabled,
		URL:        getenv("REDIS_URL", "redis://localhost:6379/0"),
		KeyPrefix:  getenv("REDIS_KEY_PREFIX", "albatextus:connect"),
		SessionTTL: ttl,
	}
	if !cfg.Enabled {
		return cfg, nil
	}
	parsed, err := url.Parse(cfg.URL)
	if err != nil || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.Host == "" {
		return Redis{}, errors.New("REDIS_URL must be a redis:// or rediss:// URL")
	}
	if cfg.SessionTTL <= 0 {
		return Redis{}, errors.New("REDIS_SESSION_TTL must be positive")
	}
	if strings.Trim(cfg.KeyPrefix, ":") == "" || strings.ContainsAny(cfg.KeyPrefix, " \t\r\n") {
		return Redis{}, errors.New("REDIS_KEY_PREFIX must be non-empty and contain no whitespace")
	}
	return cfg, nil
}
