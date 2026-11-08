// Package cachemocks holds the test doubles for the Redis commands the cache uses.
//
// The interfaces are mocked by mockery from ledgerline/internal/shared/cache; see .mockery.yaml and
// `make mocks`. The generated files are committed, so a fresh clone can
// build and test without mockery installed.
//
// The package lives in the test tree on purpose: the doubles are test
// scaffolding, not production code.
package cachemocks
