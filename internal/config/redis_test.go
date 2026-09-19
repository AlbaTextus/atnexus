package config

import (
	"testing"
	"time"
)

func TestLoadRedisDisabledByDefault(t *testing.T) {
	t.Setenv("REDIS_ENABLED", "")
	t.Setenv("REDIS_URL", "")
	cfg, err := loadRedis()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.URL != "redis://localhost:6379/0" || cfg.SessionTTL != 30*time.Second {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestLoadRedisEnabled(t *testing.T) {
	t.Setenv("REDIS_ENABLED", "true")
	t.Setenv("REDIS_URL", "rediss://cache.example.com:6380/2")
	t.Setenv("REDIS_KEY_PREFIX", "test:connect")
	t.Setenv("REDIS_SESSION_TTL", "45s")
	cfg, err := loadRedis()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.URL != "rediss://cache.example.com:6380/2" || cfg.KeyPrefix != "test:connect" || cfg.SessionTTL != 45*time.Second {
		t.Fatalf("unexpected configuration: %#v", cfg)
	}
}

func TestLoadRedisRejectsInvalidEnabledConfiguration(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"invalid enabled flag": {"REDIS_ENABLED": "sometimes"},
		"invalid URL":          {"REDIS_ENABLED": "true", "REDIS_URL": "https://cache.example.com"},
		"invalid TTL":          {"REDIS_ENABLED": "true", "REDIS_SESSION_TTL": "not-a-duration"},
		"non-positive TTL":     {"REDIS_ENABLED": "true", "REDIS_SESSION_TTL": "0s"},
		"invalid prefix":       {"REDIS_ENABLED": "true", "REDIS_KEY_PREFIX": "bad prefix"},
	} {
		t.Run(name, func(t *testing.T) {
			for key, value := range values {
				t.Setenv(key, value)
			}
			if _, err := loadRedis(); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}
