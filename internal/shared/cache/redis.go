// Package cache provides caching abstractions and implementations.
package cache

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

// RedisCache is a cache.Service implementation using Redis.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache instance.
func NewRedisCache(client *redis.Client) Service {
	return &RedisCache{client: client}
}

// Set stores a value in Redis with the specified key and TTL.
func (r *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	bytes, err := sonic.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, bytes, ttl).Err()
}

// Get retrieves a value from Redis and unmarshals it into dest.
func (r *RedisCache) Get(ctx context.Context, key string, dest any) error {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	return sonic.Unmarshal([]byte(val), dest)
}

// Delete removes a key from Redis.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// DeleteByPrefix scans for keys matching the prefix pattern and deletes them in batches.
func (r *RedisCache) DeleteByPrefix(ctx context.Context, prefix string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			pipe := r.client.Pipeline()
			pipe.Del(ctx, keys...)
			if _, err := pipe.Exec(ctx); err != nil {
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
