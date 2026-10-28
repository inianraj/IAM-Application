package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// jsonCache holds the Redis helpers shared by the cached stores. Redis errors
// are logged and treated as misses, so Postgres stays the source of truth.
type jsonCache struct {
	rdb    *redis.Client
	ttl    time.Duration
	logger *slog.Logger
}

func (c *jsonCache) get(ctx context.Context, key string, dst any) bool {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			c.logger.Warn("cache read failed", "key", key, "error", err)
		}
		return false
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		c.logger.Warn("cache decode failed", "key", key, "error", err)
		return false
	}
	return true
}

func (c *jsonCache) set(ctx context.Context, key string, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		c.logger.Warn("cache encode failed", "key", key, "error", err)
		return
	}
	if err := c.rdb.Set(ctx, key, raw, c.ttl).Err(); err != nil {
		c.logger.Warn("cache write failed", "key", key, "error", err)
	}
}

func (c *jsonCache) invalidate(ctx context.Context, keys ...string) {
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		c.logger.Warn("cache invalidation failed; relying on TTL", "keys", keys, "error", err)
	}
}
