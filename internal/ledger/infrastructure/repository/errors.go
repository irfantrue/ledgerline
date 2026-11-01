package repository

import "errors"

// Sentinel errors returned by the ledger repositories.
//
// The repository methods wrap these with additional context, so callers must
// detect them with errors.Is rather than a direct comparison.
var (
	// ErrAccountNotFound is returned when the requested account does not exist.
	ErrAccountNotFound = errors.New("account not found")

	// ErrJournalNotFound is returned when the requested journal does not exist.
	ErrJournalNotFound = errors.New("journal not found")

	// ErrPostingNotFound is returned when the requested posting does not exist.
	ErrPostingNotFound = errors.New("posting not found")
)
