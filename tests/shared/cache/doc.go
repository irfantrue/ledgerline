// Package cachetest holds the tests for internal/shared/cache.
//
// The tests live outside internal/ so the production package stays free of
// test-only helpers and fixtures.
//
// The tests need no Redis: the cache is exercised against a mocked backend.
package cachetest
