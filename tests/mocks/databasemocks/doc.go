// Package databasemocks holds the test doubles for the database handle and the connection pool.
//
// The interfaces are mocked by mockery from ledgerline/internal/shared/database; see .mockery.yaml and
// `make mocks`. The generated files are committed, so a fresh clone can
// build and test without mockery installed.
//
// The package lives in the test tree on purpose: the doubles are test
// scaffolding, not production code.
package databasemocks
