package ledger

import (
	"context"
	"errors"
	"testing"

	"ledgerline/internal/ledger/infrastructure/repository"
	"ledgerline/internal/shared/database"
	"ledgerline/tests/mocks/databasemocks"
	"ledgerline/tests/mocks/pgxmocks"
	"ledgerline/tests/mocks/repositorymocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// errStubDB is the error every stub handle returns.
var errStubDB = errors.New("stub database handle")

// TestRepositories_ResolveTheHandlePerCall checks that a repository asks its
// querier for the database handle on every call instead of holding on to one.
//
// That is the mechanism that lets a call join the transaction its context
// carries: the handle of a call depends on the context of that call, so a
// journal and its postings can be written in one transaction - which the
// deferred balance trigger of the ledger requires.
func TestRepositories_ResolveTheHandlePerCall(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	tests := []struct {
		name string
		call func(q repository.Querier) error
	}{
		{
			name: "journal",
			call: func(q repository.Querier) error {
				_, err := repository.NewJournalRepository(q).GetByID(ctx, id)
				return err
			},
		},
		{
			name: "account",
			call: func(q repository.Querier) error {
				_, err := repository.NewAccountRepository(q).GetByID(ctx, id)
				return err
			},
		},
		{
			name: "posting",
			call: func(q repository.Querier) error {
				_, err := repository.NewPostingRepository(q).GetByID(ctx, id)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The handle fails every read, so the error of the stub proves
			// that the repository used the handle it was given.
			db := &databasemocks.DBTX{}
			db.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
				Return(pgxmocks.ErrRow(errStubDB)).
				Once()

			querier := &repositorymocks.Querier{}
			querier.On("Querier", mock.Anything).Return(db)

			err := tt.call(querier)

			require.ErrorIs(t, err, errStubDB)
			querier.AssertNumberOfCalls(t, "Querier", 1)
			db.AssertExpectations(t)
		})
	}
}

// TestTxManager_QuerierPrefersTheTransaction checks the other half of the
// mechanism: the handle a repository gets is the transaction of the context
// when there is one, and the pool otherwise.
func TestTxManager_QuerierPrefersTheTransaction(t *testing.T) {
	tm := database.NewTxManager(nil)

	tx := &pgxmocks.Tx{}
	ctx := database.ContextWithTx(context.Background(), tx)

	require.Same(t, tx, tm.Querier(ctx))
}
