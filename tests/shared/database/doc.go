// Package databasetest holds the tests for internal/shared/database.
//
// The tests live outside internal/ so the production package stays free of
// test-only helpers and fixtures.
//
// The tests need no database: the transaction manager is exercised against a
// mocked pool that hands out a mocked transaction.
package databasetest
