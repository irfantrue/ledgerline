package cachetest

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"ledgerline/internal/shared/cache"

	"github.com/redis/go-redis/v9"
)

// newCache returns a cache backed by Redis, or skips the test.
func newCache(t *testing.T) cache.Service {
	t.Helper()

	host := os.Getenv("REDIS_HOST")
	if host == "" {
		t.Skip("REDIS_HOST not set; skipping integration test")
	}

	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     host + ":" + port,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	t.Cleanup(func() { _ = client.Close() })

	return cache.NewRedisCache(client)
}

// key returns a key unique to the running test.
func key(t *testing.T, name string) string {
	t.Helper()
	return "cache.test." + name + "." + t.Name()
}

func TestSetAndGet(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	want := payload{Name: "alice", Count: 3}
	k := key(t, "setget")

	if err := c.Set(ctx, k, want, time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	var got payload
	if err := c.Get(ctx, k, &got); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}

func TestGet_MissingKey(t *testing.T) {
	c := newCache(t)

	var got string
	err := c.Get(context.Background(), key(t, "missing"), &got)
	if err == nil {
		t.Fatal("Get() error = nil, want an error for a missing key")
	}
}

func TestDelete(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	k := key(t, "delete")
	if err := c.Set(ctx, k, "value", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := c.Delete(ctx, k); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	var got string
	if err := c.Get(ctx, k, &got); err == nil {
		t.Fatal("Get() error = nil, want an error after the key was deleted")
	}
}

// TestDeleteByPrefix checks that only the keys with the prefix are removed.
func TestDeleteByPrefix(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	prefix := key(t, "prefix")
	keep := key(t, "keep")

	for _, k := range []string{prefix + ".1", prefix + ".2"} {
		if err := c.Set(ctx, k, "value", time.Minute); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}
	if err := c.Set(ctx, keep, "value", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := c.DeleteByPrefix(ctx, prefix); err != nil {
		t.Fatalf("DeleteByPrefix() error = %v", err)
	}

	for _, k := range []string{prefix + ".1", prefix + ".2"} {
		var got string
		if err := c.Get(ctx, k, &got); err == nil {
			t.Errorf("key %q still exists after DeleteByPrefix", k)
		}
	}

	var got string
	if err := c.Get(ctx, keep, &got); err != nil {
		t.Errorf("key %q was removed but should have been kept: %v", keep, err)
	}
}

// TestDeleteByPrefix_ManyKeys exercises the batched scan loop by writing more
// keys than fit into a single batch.
func TestDeleteByPrefix_ManyKeys(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	prefix := key(t, "many")
	for i := 0; i < 150; i++ {
		k := prefix + "." + strconv.Itoa(i)
		if err := c.Set(ctx, k, "value", time.Minute); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	if err := c.DeleteByPrefix(ctx, prefix); err != nil {
		t.Fatalf("DeleteByPrefix() error = %v", err)
	}

	var got string
	if err := c.Get(ctx, prefix+".0", &got); err == nil {
		t.Error("key still exists after DeleteByPrefix")
	}
}

func TestSet_UnmarshalableValue(t *testing.T) {
	c := newCache(t)

	// Channels cannot be marshalled to JSON.
	if err := c.Set(context.Background(), key(t, "bad"), make(chan int), time.Minute); err == nil {
		t.Fatal("Set() error = nil, want a marshalling error")
	}
}

func TestGet_InvalidPayload(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	k := key(t, "invalid")
	// Write a value that is not valid JSON for the destination type.
	if err := c.Set(ctx, k, "not json", time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	var got struct {
		Name string `json:"name"`
	}
	if err := c.Get(ctx, k, &got); err == nil {
		t.Fatal("Get() error = nil, want an unmarshalling error")
	}
}
