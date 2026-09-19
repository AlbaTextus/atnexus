package store

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
)

func TestRedisSessionCacheLifecycle(t *testing.T) {
	server := miniredis.RunT(t)
	cache, err := newRedisSessionCache(context.Background(), "redis://"+server.Addr()+"/0", "test:connect", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.close)

	ctx := context.Background()
	userID := uuid.New()
	want := Session{ID: uuid.New(), UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}
	_, _, version := cache.get(ctx, "token")
	cache.set(ctx, "token", want, version)
	got, ok, _ := cache.get(ctx, "token")
	if !ok || got.ID != want.ID || got.UserID != want.UserID || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("unexpected cached session: %#v, hit=%v", got, ok)
	}

	cache.invalidateUser(ctx, userID)
	if _, ok, _ := cache.get(ctx, "token"); ok {
		t.Fatal("user invalidation must make existing sessions miss")
	}

	_, _, version = cache.get(ctx, "token")
	cache.set(ctx, "token", want, version)
	if _, ok, _ := cache.get(ctx, "token"); !ok {
		t.Fatal("session should be cacheable after invalidation")
	}
	cache.delete(ctx, "token")
	if _, ok, _ := cache.get(ctx, "token"); ok {
		t.Fatal("deleted session must miss")
	}
}

func TestRedisSessionCacheCapsTTLAtSessionExpiry(t *testing.T) {
	server := miniredis.RunT(t)
	cache, err := newRedisSessionCache(context.Background(), "redis://"+server.Addr()+"/0", "test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.close)

	session := Session{ID: uuid.New(), UserID: uuid.New(), ExpiresAt: time.Now().Add(20 * time.Second)}
	ctx := context.Background()
	_, _, version := cache.get(ctx, "short")
	cache.set(ctx, "short", session, version)
	ttl := server.TTL(cache.sessionKey("short"))
	if ttl <= 0 || ttl > 20*time.Second {
		t.Fatalf("cache TTL %s exceeds session lifetime", ttl)
	}
	server.FastForward(21 * time.Second)
	if _, ok, _ := cache.get(context.Background(), "short"); ok {
		t.Fatal("expired session must not be returned")
	}
}

func TestRedisSessionCacheDoesNotResurrectInvalidatedSnapshot(t *testing.T) {
	server := miniredis.RunT(t)
	cache, err := newRedisSessionCache(context.Background(), "redis://"+server.Addr()+"/0", "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.close)

	ctx := context.Background()
	session := Session{ID: uuid.New(), UserID: uuid.New(), ExpiresAt: time.Now().Add(time.Hour)}
	_, _, snapshot := cache.get(ctx, "bulk")
	cache.invalidateUser(ctx, session.UserID)
	cache.set(ctx, "bulk", session, snapshot)
	if _, ok, _ := cache.get(ctx, "bulk"); ok {
		t.Fatal("a pre-invalidation database snapshot must not be cached")
	}

	_, _, snapshot = cache.get(ctx, "single")
	cache.delete(ctx, "single")
	cache.set(ctx, "single", session, snapshot)
	if _, ok, _ := cache.get(ctx, "single"); ok {
		t.Fatal("a pre-revocation database snapshot must not be cached")
	}
}
