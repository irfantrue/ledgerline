package middleware

import (
	"sync"
	"time"

	"ledgerline/pkg/ctxval"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/singleflight"
)

const (
	maxCacheEntries = 1000
	cacheTTL        = 10 * time.Minute
)

type cacheEntry struct {
	loc      *time.Location
	loadedAt time.Time
}

type boundedCache struct {
	mu    sync.Mutex
	data  map[string]*cacheEntry
	order []string // simple LRU tracking
}

func newBoundedCache() *boundedCache {
	return &boundedCache{
		data:  make(map[string]*cacheEntry),
		order: make([]string, 0, maxCacheEntries),
	}
}

func (c *boundedCache) Get(key string) (*time.Location, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.data[key]
	if !ok {
		return nil, false
	}

	if time.Since(entry.loadedAt) > cacheTTL {
		delete(c.data, key)
		c.removeFromOrder(key)
		return nil, false
	}

	c.moveToEnd(key)
	return entry.loc, true
}

func (c *boundedCache) Set(key string, loc *time.Location) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.order) >= maxCacheEntries {
		oldest := c.order[0]
		delete(c.data, oldest)
		c.order = c.order[1:]
	}

	c.data[key] = &cacheEntry{
		loc:      loc,
		loadedAt: time.Now(),
	}
	c.order = append(c.order, key)
}

func (c *boundedCache) removeFromOrder(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

func (c *boundedCache) moveToEnd(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, key)
			return
		}
	}
}

var (
	locCache  = newBoundedCache()
	loadGroup singleflight.Group
)

// LoadLocation resolves a timezone name to a *time.Location. Results are
// cached, and the second return value reports whether the name is valid.
func LoadLocation(name string) (*time.Location, bool) {
	if loc, ok := locCache.Get(name); ok {
		return loc, true
	}

	v, err, _ := loadGroup.Do(name, func() (any, error) {
		// Double-check after acquiring singleflight
		if loc, ok := locCache.Get(name); ok {
			return loc, nil
		}

		loc, err := time.LoadLocation(name)
		if err != nil {
			return nil, err
		}

		locCache.Set(name, loc)
		return loc, nil
	})

	if err != nil {
		return nil, false
	}

	return v.(*time.Location), true
}

// NewTimezone creates a middleware that parses the "X-Timezone" header.
// It sets the parsed timezone (or the default timezone if invalid/missing) into the request context.
func NewTimezone(defaultTimezone string) fiber.Handler {
	defaultLoc, ok := LoadLocation(defaultTimezone)
	if !ok {
		defaultLoc = time.UTC
	}

	return func(c fiber.Ctx) error {
		loc := defaultLoc

		if tzHeader := c.Get("X-Timezone"); tzHeader != "" {
			if parsed, ok := LoadLocation(tzHeader); ok {
				loc = parsed
			}
		}

		c.SetContext(ctxval.SetTimezone(c, loc))

		return c.Next()
	}
}
