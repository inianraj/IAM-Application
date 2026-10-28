package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/redis/go-redis/v9"
	"time"
)

const (
	// MaxLoginFailures is how many failed logins an email+IP pair may make in
	// LoginFailureWindow before further attempts are refused.
	MaxLoginFailures   = 5
	LoginFailureWindow = 15 * time.Minute
)

// LoginLimiter counts failed logins per email and IP in Redis.
type LoginLimiter struct {
	rdb *redis.Client
}

func NewLoginLimiter(rdb *redis.Client) *LoginLimiter {
	return &LoginLimiter{rdb: rdb}
}

// The email is hashed so the key is a fixed shape and holds no address.
func failKey(email, ip string) string {
	sum := sha256.Sum256([]byte(email))
	return keyPrefix + "login_fail:" + hex.EncodeToString(sum[:]) + ":" + ip
}

// Blocked reports whether this email+IP has used up its failures.
func (l *LoginLimiter) Blocked(ctx context.Context, email, ip string) (bool, error) {
	n, err := l.rdb.Get(ctx, failKey(email, ip)).Int()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read login failures: %w", err)
	}
	return n >= MaxLoginFailures, nil
}

// Fail records one failed attempt; the window starts at the first failure.
func (l *LoginLimiter) Fail(ctx context.Context, email, ip string) error {
	key := failKey(email, ip)

	n, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("count login failure: %w", err)
	}
	if n == 1 {
		if err := l.rdb.Expire(ctx, key, LoginFailureWindow).Err(); err != nil {
			return fmt.Errorf("expire login failures: %w", err)
		}
	}
	return nil
}

// Reset forgets earlier failures after a successful login.
func (l *LoginLimiter) Reset(ctx context.Context, email, ip string) error {
	if err := l.rdb.Del(ctx, failKey(email, ip)).Err(); err != nil {
		return fmt.Errorf("reset login failures: %w", err)
	}
	return nil
}
