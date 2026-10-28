// Package session keeps login sessions in Redis.
//
// The browser holds a random token; Redis is keyed by the SHA-256 of that
// token, so a Redis leak does not reveal usable tokens.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyPrefix = "crud:v1:"

	// maxTokenLength rejects absurd cookie values before hashing them.
	maxTokenLength = 128
)

// ErrNotFound means the token is unknown or the session has expired. Any other
// error from a Store means Redis could not be reached.
var ErrNotFound = errors.New("session not found")

// Data is what a session remembers. It proves identity only; roles and groups
// are read fresh on each request.
type Data struct {
	UserID         int64     `json:"user_id"`
	OrganisationID int64     `json:"organisation_id"`
	CreatedAt      time.Time `json:"created_at"`
	IP             string    `json:"ip"`
	UserAgent      string    `json:"user_agent"`
}

// Store creates, reads and ends sessions.
type Store interface {
	// Create stores the session and returns the raw token for the cookie.
	Create(ctx context.Context, d Data) (token string, err error)
	Get(ctx context.Context, token string) (Data, error)
	Delete(ctx context.Context, token string) error
}

// RedisStore is a Store whose sessions expire ttl after creation.
type RedisStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisStore(rdb *redis.Client, ttl time.Duration) *RedisStore {
	return &RedisStore{rdb: rdb, ttl: ttl}
}

// hashToken is the Redis-side identity of a token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func sessionKey(token string) string {
	return keyPrefix + "session:" + hashToken(token)
}

func (s *RedisStore) Create(ctx context.Context, d Data) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])

	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	value, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("encode session: %w", err)
	}

	if err := s.rdb.Set(ctx, sessionKey(token), value, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("store session: %w", err)
	}
	return token, nil
}

func (s *RedisStore) Get(ctx context.Context, token string) (Data, error) {
	if token == "" || len(token) > maxTokenLength {
		return Data{}, ErrNotFound
	}

	raw, err := s.rdb.Get(ctx, sessionKey(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Data{}, ErrNotFound
	}
	if err != nil {
		return Data{}, fmt.Errorf("read session: %w", err)
	}

	var d Data
	if err := json.Unmarshal(raw, &d); err != nil {
		return Data{}, ErrNotFound
	}
	return d, nil
}

func (s *RedisStore) Delete(ctx context.Context, token string) error {
	if token == "" || len(token) > maxTokenLength {
		return nil
	}
	if err := s.rdb.Del(ctx, sessionKey(token)).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
