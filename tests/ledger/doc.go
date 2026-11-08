// Package ledger holds the tests for the ledger module.
//
// The tests live outside internal/ so the production packages stay free of
// test-only helpers and fixtures. Only the exported API of the ledger module
// is exercised from here.
//
// The tests need no database: the repositories are exercised against a
// mocked database handle, and the invariants the database enforces are pinned
// down by reading the migrations.
package ledger
