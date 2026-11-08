// Package pgxmocks holds the test doubles for the pgx driver types the doubles stand in for.
//
// The interfaces are mocked by mockery from github.com/jackc/pgx/v5; see .mockery.yaml and
// `make mocks`. The generated files are committed, so a fresh clone can
// build and test without mockery installed.
//
// The package lives in the test tree on purpose: the doubles are test
// scaffolding, not production code.
package pgxmocks
