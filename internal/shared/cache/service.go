package cache

import (
	"context"
	"time"
)

// Service defines the contract for caching operations.
type Service interface {
	// Set data in the cache with a Time To Live (TTL)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Get retrieves data from the cache and places it into the 'dest' parameter (pointer).
	Get(ctx context.Context, key string, dest any) error

	// Delete removes data from the cache (used during invalidation)
	Delete(ctx context.Context, key string) error

	// DeleteByPrefix deletes multiple keys at once (useful for clearing an entire product list cache)
	DeleteByPrefix(ctx context.Context, prefix string) error
}
