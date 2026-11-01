// Package cachetest holds the tests for internal/shared/cache.
//
// The tests live outside internal/ so the production package stays free of
// test-only helpers and fixtures.
//
// The integration tests need a real Redis and are skipped unless REDIS_HOST
// is set.
package cachetest
