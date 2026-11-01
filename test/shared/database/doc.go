// Package databasetest holds the tests for internal/shared/database.
//
// The tests live outside internal/ so the production package stays free of
// test-only helpers and fixtures.
//
// The integration tests need a real PostgreSQL and are skipped unless DB_DSN
// is set. See tx_manager_integration_test.go for the details.
package databasetest
