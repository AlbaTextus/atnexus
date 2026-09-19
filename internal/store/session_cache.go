package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type cachedSession struct {
	Session       Session `json:"session"`
	GlobalVersion int64   `json:"global_version"`
	TokenVersion  int64   `json:"token_version"`
}

type sessionCacheVersion struct{ global, token int64 }

type redisSessionCache struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

func newRedisSessionCache(ctx context.Context, rawURL, prefix string, ttl time.Duration) (*redisSessionCache, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	return &redisSessionCache{client: client, prefix: strings.TrimSuffix(prefix, ":"), ttl: ttl}, nil
}

func (c *redisSessionCache) sessionKey(tokenHash string) string {
	return c.prefix + ":session:" + tokenHash
}

func (c *redisSessionCache) globalVersionKey() string {
	return c.prefix + ":session-version"
}

func (c *redisSessionCache) tokenVersionKey(tokenHash string) string {
	return c.prefix + ":session-token-version:" + tokenHash
}

func cachedInt(value any) int64 {
	if value == nil {
		return 0
	}
	text := fmt.Sprint(value)
	if bytes, ok := value.([]byte); ok {
		text = string(bytes)
	}
	n, _ := strconv.ParseInt(text, 10, 64)
	return n
}

func (c *redisSessionCache) versions(ctx context.Context, tokenHash string) (sessionCacheVersion, error) {
	values, err := c.client.MGet(ctx, c.globalVersionKey(), c.tokenVersionKey(tokenHash)).Result()
	if err != nil {
		return sessionCacheVersion{}, err
	}
	return sessionCacheVersion{global: cachedInt(values[0]), token: cachedInt(values[1])}, nil
}

func (c *redisSessionCache) get(ctx context.Context, tokenHash string) (Session, bool, sessionCacheVersion) {
	key := c.sessionKey(tokenHash)
	values, err := c.client.MGet(ctx, key, c.globalVersionKey(), c.tokenVersionKey(tokenHash)).Result()
	if err != nil {
		return Session{}, false, sessionCacheVersion{}
	}
	version := sessionCacheVersion{global: cachedInt(values[1]), token: cachedInt(values[2])}
	if values[0] == nil {
		return Session{}, false, version
	}
	var data []byte
	switch value := values[0].(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	default:
		_ = c.client.Del(ctx, key).Err()
		return Session{}, false, version
	}
	var entry cachedSession
	if json.Unmarshal(data, &entry) != nil || entry.Session.UserID == uuid.Nil || entry.Session.RevokedAt != nil || !entry.Session.ExpiresAt.After(time.Now()) {
		_ = c.client.Del(ctx, key).Err()
		return Session{}, false, version
	}
	if entry.GlobalVersion != version.global || entry.TokenVersion != version.token {
		_ = c.client.Del(ctx, key).Err()
		return Session{}, false, version
	}
	return entry.Session, true, version
}

func (c *redisSessionCache) set(ctx context.Context, tokenHash string, session Session, version sessionCacheVersion) {
	ttl := min(c.ttl, time.Until(session.ExpiresAt))
	if ttl <= 0 || session.RevokedAt != nil {
		return
	}
	data, err := json.Marshal(cachedSession{Session: session, GlobalVersion: version.global, TokenVersion: version.token})
	if err == nil {
		_ = c.client.Set(ctx, c.sessionKey(tokenHash), data, ttl).Err()
	}
	current, err := c.versions(ctx, tokenHash)
	if err != nil || current != version {
		_ = c.client.Del(ctx, c.sessionKey(tokenHash)).Err()
	}
}

func (c *redisSessionCache) delete(ctx context.Context, tokenHash string) {
	versionKey := c.tokenVersionKey(tokenHash)
	pipe := c.client.TxPipeline()
	pipe.Incr(ctx, versionKey)
	pipe.Expire(ctx, versionKey, 2*c.ttl)
	pipe.Del(ctx, c.sessionKey(tokenHash))
	_, _ = pipe.Exec(ctx)
}

func (c *redisSessionCache) invalidateUser(ctx context.Context, _ uuid.UUID) {
	_ = c.client.Incr(ctx, c.globalVersionKey()).Err()
}

func (c *redisSessionCache) close() { _ = c.client.Close() }
