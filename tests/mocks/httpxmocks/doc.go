// Package httpxmocks holds the test doubles for the database handle the readiness probe pings.
//
// The interfaces are mocked by mockery from ledgerline/internal/shared/httpx; see .mockery.yaml and
// `make mocks`. The generated files are committed, so a fresh clone can
// build and test without mockery installed.
//
// The package lives in the test tree on purpose: the doubles are test
// scaffolding, not production code.
package httpxmocks
