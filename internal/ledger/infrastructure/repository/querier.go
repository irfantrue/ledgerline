package repository

import (
	"context"

	"ledgerline/internal/shared/database"
)

// Querier resolves the database handle a repository uses for a single call.
//
// The handle is resolved per call instead of per repository, so a call joins
// the transaction that its context carries. That is what makes it possible to
// write a journal and its postings in one transaction, which the deferred
// balance trigger of the ledger requires.
//
// database.TxManager implements this interface.
type Querier interface {
	Querier(ctx context.Context) database.DBTX
}
