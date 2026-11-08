package cache

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

// RedisCache is a cache.Service implementation using Redis.
type RedisCache struct {
	backend Backend
}

// NewRedisCache creates a new Redis cache instance.
func NewRedisCache(client *redis.Client) Service {
	return NewRedisCacheWithBackend(NewRedisBackend(client))
}

// NewRedisCacheWithBackend creates a cache on top of a Backend.
//
// The Backend indirection exists so the cache can be tested without a running
// Redis: the marshalling, the key handling and the prefix scan all live in
// RedisCache, and only the commands themselves come from the backend.
func NewRedisCacheWithBackend(backend Backend) Service {
	return &RedisCache{backend: backend}
}

// Set stores a value in Redis with the specified key and TTL.
func (r *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	bytes, err := sonic.Marshal(value)
	if err != nil {
		return err
	}
	return r.backend.Set(ctx, key, bytes, ttl)
}

// Get retrieves a value from Redis and unmarshals it into dest.
func (r *RedisCache) Get(ctx context.Context, key string, dest any) error {
	val, err := r.backend.Get(ctx, key)
	if err != nil {
		return err
	}
	return sonic.Unmarshal(val, dest)
}

// Delete removes a key from Redis.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.backend.Del(ctx, key)
}

// DeleteByPrefix scans for keys matching the prefix pattern and deletes them in batches.
func (r *RedisCache) DeleteByPrefix(ctx context.Context, prefix string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := r.backend.Scan(ctx, cursor, prefix+"*", 100)
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := r.backend.Del(ctx, keys...); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
