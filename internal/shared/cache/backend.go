package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Backend is the subset of the Redis commands the cache needs.
//
// The interface exists so the cache can be tested without a running Redis.
// The values are raw bytes: the marshalling belongs to the cache, not to the
// backend.
type Backend interface {
	// Set stores a value under a key with the given TTL.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Get returns the raw value stored under a key.
	Get(ctx context.Context, key string) ([]byte, error)

	// Del removes the given keys.
	Del(ctx context.Context, keys ...string) error

	// Scan returns the keys matching a pattern, starting at the cursor.
	// A returned cursor of 0 means the scan is complete.
	Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error)
}

// redisBackend implements Backend on top of a Redis client.
type redisBackend struct {
	client *redis.Client
}

// NewRedisBackend returns a Backend backed by a Redis client.
func NewRedisBackend(client *redis.Client) Backend {
	return &redisBackend{client: client}
}

// Set stores a value under a key with the given TTL.
func (b *redisBackend) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return b.client.Set(ctx, key, value, ttl).Err()
}

// Get returns the raw value stored under a key.
func (b *redisBackend) Get(ctx context.Context, key string) ([]byte, error) {
	return b.client.Get(ctx, key).Bytes()
}

// Del removes the given keys.
func (b *redisBackend) Del(ctx context.Context, keys ...string) error {
	return b.client.Del(ctx, keys...).Err()
}

// Scan returns the keys matching a pattern, starting at the cursor.
func (b *redisBackend) Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return b.client.Scan(ctx, cursor, match, count).Result()
}
