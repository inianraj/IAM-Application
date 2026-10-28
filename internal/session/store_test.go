package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

func TestCreateStoresOnlyTheTokenHash(t *testing.T) {
	ctx := context.Background()
	rdb, mr := newRedis(t)
	s := NewRedisStore(rdb, time.Hour)

	token, err := s.Create(ctx, Data{UserID: 7, OrganisationID: 3, IP: "10.0.0.1", UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 {
		t.Fatalf("token too short to be 256 bits: %q", token)
	}

	keys := mr.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected one key, got %v", keys)
	}
	if strings.Contains(keys[0], token) {
		t.Fatal("the raw token must not appear in the Redis key")
	}
	if keys[0] != sessionKey(token) {
		t.Fatalf("key = %q, want hash of token", keys[0])
	}
	if ttl := mr.TTL(keys[0]); ttl <= 0 || ttl > time.Hour {
		t.Fatalf("unexpected TTL %v", ttl)
	}
}

func TestGetReturnsDataAndRejectsUnknownTokens(t *testing.T) {
	ctx := context.Background()
	rdb, _ := newRedis(t)
	s := NewRedisStore(rdb, time.Hour)

	token, _ := s.Create(ctx, Data{UserID: 7, OrganisationID: 3})

	got, err := s.Get(ctx, token)
	if err != nil || got.UserID != 7 || got.OrganisationID != 3 || got.CreatedAt.IsZero() {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	for _, bad := range []string{"", "nope", strings.Repeat("a", 500)} {
		if _, err := s.Get(ctx, bad); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get(%q) = %v, want ErrNotFound", bad, err)
		}
	}
}

func TestSessionsExpireAndDelete(t *testing.T) {
	ctx := context.Background()
	rdb, mr := newRedis(t)
	s := NewRedisStore(rdb, time.Hour)

	expiring, _ := s.Create(ctx, Data{UserID: 1})
	mr.FastForward(2 * time.Hour)
	if _, err := s.Get(ctx, expiring); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: %v", err)
	}

	token, _ := s.Create(ctx, Data{UserID: 1})
	if err := s.Delete(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session: %v", err)
	}
	if err := s.Delete(ctx, "unknown"); err != nil {
		t.Fatalf("deleting an unknown token must not fail: %v", err)
	}
}

func TestRedisDownIsNotReportedAsNotFound(t *testing.T) {
	ctx := context.Background()
	rdb, mr := newRedis(t)
	s := NewRedisStore(rdb, time.Hour)
	token, _ := s.Create(ctx, Data{UserID: 1})

	mr.Close()

	_, err := s.Get(ctx, token)
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("a Redis failure must be distinguishable from an unknown session, got %v", err)
	}
}

func TestLoginLimiter(t *testing.T) {
	ctx := context.Background()
	rdb, mr := newRedis(t)
	l := NewLoginLimiter(rdb)

	for i := 0; i < MaxLoginFailures; i++ {
		if blocked, _ := l.Blocked(ctx, "a@corp.com", "1.1.1.1"); blocked {
			t.Fatalf("blocked after only %d failures", i)
		}
		if err := l.Fail(ctx, "a@corp.com", "1.1.1.1"); err != nil {
			t.Fatal(err)
		}
	}
	if blocked, _ := l.Blocked(ctx, "a@corp.com", "1.1.1.1"); !blocked {
		t.Fatalf("expected a block after %d failures", MaxLoginFailures)
	}

	// Other emails and other IPs are counted separately.
	if blocked, _ := l.Blocked(ctx, "b@corp.com", "1.1.1.1"); blocked {
		t.Fatal("another email must not be blocked")
	}
	if blocked, _ := l.Blocked(ctx, "a@corp.com", "2.2.2.2"); blocked {
		t.Fatal("another IP must not be blocked")
	}

	for _, k := range mr.Keys() {
		if strings.Contains(k, "a@corp.com") {
			t.Fatalf("the email must be hashed in the key: %s", k)
		}
	}

	mr.FastForward(LoginFailureWindow + time.Second)
	if blocked, _ := l.Blocked(ctx, "a@corp.com", "1.1.1.1"); blocked {
		t.Fatal("the block must end with the window")
	}

	_ = l.Fail(ctx, "a@corp.com", "1.1.1.1")
	if err := l.Reset(ctx, "a@corp.com", "1.1.1.1"); err != nil {
		t.Fatal(err)
	}
	if mr.Exists(failKey("a@corp.com", "1.1.1.1")) {
		t.Fatal("Reset must delete the counter")
	}
}
