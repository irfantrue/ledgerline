package ledger

import (
	"context"
	"errors"
	"testing"

	"ledgerline/internal/ledger/infrastructure/repository"
	"ledgerline/internal/shared/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// errStubDB is the error every stub handle returns.
var errStubDB = errors.New("stub database handle")

// stubDBTX is a database handle that fails every call. It is enough to prove
// that a repository asked its querier for a handle and used the one it got.
type stubDBTX struct{}

func (stubDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errStubDB
}

func (stubDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errStubDB
}

func (stubDBTX) QueryRow(context.Context, string, ...any) pgx.Row {
	return stubRow{}
}

// stubRow is the pgx.Row of the stub handle.
type stubRow struct{}

func (stubRow) Scan(...any) error { return errStubDB }

// countingQuerier counts how often a repository asked for a handle.
type countingQuerier struct {
	calls int
}

func (c *countingQuerier) Querier(context.Context) database.DBTX {
	c.calls++
	return stubDBTX{}
}

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
			querier := &countingQuerier{}

			err := tt.call(querier)
			if !errors.Is(err, errStubDB) {
				t.Errorf("error = %v, want the error of the stub handle", err)
			}
			if querier.calls != 1 {
				t.Errorf("the querier was asked %d times, want 1", querier.calls)
			}
		})
	}
}

// TestTxManager_QuerierPrefersTheTransaction checks the other half of the
// mechanism: the handle a repository gets is the transaction of the context
// when there is one, and the pool otherwise.
func TestTxManager_QuerierPrefersTheTransaction(t *testing.T) {
	tm := database.NewTxManager(nil)

	tx := &stubTx{}
	ctx := database.ContextWithTx(context.Background(), tx)

	if got := tm.Querier(ctx); got != database.DBTX(tx) {
		t.Errorf("Querier() = %v, want the transaction of the context", got)
	}
}

// stubTx stands in for a transaction. It only has to be distinguishable, the
// test never calls it.
type stubTx struct {
	pgx.Tx
}
