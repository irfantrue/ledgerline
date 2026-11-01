// Package ledger holds the tests for the ledger module.
//
// The tests live outside internal/ so the production packages stay free of
// test-only helpers and fixtures. Only the exported API of the ledger module
// is exercised from here.
//
// The integration tests need a real PostgreSQL and are skipped unless DB_DSN
// is set. See repository_integration_test.go for the details.
package ledger
