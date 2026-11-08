package databasetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"ledgerline/internal/shared/database"
	"ledgerline/tests/mocks/databasemocks"
	"ledgerline/tests/mocks/pgxmocks"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTxFrom_NoTx(t *testing.T) {
	_, ok := database.TxFrom(context.Background())
	require.False(t, ok, "TxFrom on an empty context should not find a transaction")
}

// Regression: TxFrom used to assert pgxpool.Tx, but the stored value is a
// pgx.Tx, so the assertion always failed and transactions were never reused.
func TestTxFrom_ReturnsStoredTx(t *testing.T) {
	want := &pgxmocks.Tx{}

	got, ok := database.TxFrom(database.ContextWithTx(context.Background(), want))
	require.True(t, ok, "TxFrom should find the stored pgx.Tx")
	require.Same(t, pgx.Tx(want), got)
}

func TestDo_ReusesExistingTx(t *testing.T) {
	want := &pgxmocks.Tx{}
	m := database.NewTxManager(nil) // nil pool: must not be used when a tx already exists

	called := false
	err := m.Do(database.ContextWithTx(context.Background(), want), func(ctx context.Context) error {
		called = true
		got, ok := database.TxFrom(ctx)
		require.True(t, ok, "the inner context should carry the transaction")
		require.Same(t, pgx.Tx(want), got)
		return nil
	})

	require.NoError(t, err)
	require.True(t, called, "the function should have been called")
}

// TestDo_ReusedTxPropagatesErrorWithoutRollback checks that a transaction
// that was handed in is not rolled back by the manager: the outer owner
// decides what happens to it.
func TestDo_ReusedTxPropagatesErrorWithoutRollback(t *testing.T) {
	errBoom := errors.New("boom")
	m := database.NewTxManager(nil)

	err := m.Do(database.ContextWithTx(context.Background(), &pgxmocks.Tx{}), func(context.Context) error {
		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
}

func TestQuerier_PrefersTx(t *testing.T) {
	tx := &pgxmocks.Tx{}
	m := database.NewTxManager(nil)

	require.Same(t, database.DBTX(tx), m.Querier(database.ContextWithTx(context.Background(), tx)))
}

// TestDo_CommitsOnSuccess checks that a successful function commits the
// transaction the manager opened.
func TestDo_CommitsOnSuccess(t *testing.T) {
	tx := &pgxmocks.Tx{}
	tx.On("Commit", mock.Anything).Return(nil).Once()

	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, mock.Anything).Return(tx, nil).Once()

	m := database.NewTxManager(pool)

	err := m.Do(context.Background(), func(context.Context) error { return nil })

	require.NoError(t, err)
	tx.AssertExpectations(t)
	pool.AssertExpectations(t)
}

// TestDo_RollsBackOnError checks that a failing function rolls the
// transaction back instead of leaving it open.
func TestDo_RollsBackOnError(t *testing.T) {
	errBoom := errors.New("boom")

	tx := &pgxmocks.Tx{}
	tx.On("Commit", mock.Anything).Return(nil).Maybe()
	tx.On("Rollback", mock.Anything).Return(nil).Once()

	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, mock.Anything).Return(tx, nil).Once()

	m := database.NewTxManager(pool)

	err := m.Do(context.Background(), func(context.Context) error { return errBoom })

	require.ErrorIs(t, err, errBoom)
	tx.AssertExpectations(t)
}

// TestDo_PropagatesCommitError checks that a commit that is rejected by the
// database is reported to the caller. The commit is where a deferred
// constraint (a journal that does not balance) is rejected.
func TestDo_PropagatesCommitError(t *testing.T) {
	errCommit := errors.New("commit failed")

	tx := &pgxmocks.Tx{}
	tx.On("Commit", mock.Anything).Return(errCommit).Once()
	tx.On("Rollback", mock.Anything).Return(nil).Maybe()

	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, mock.Anything).Return(tx, nil).Once()

	m := database.NewTxManager(pool)

	err := m.Do(context.Background(), func(context.Context) error { return nil })

	require.ErrorIs(t, err, errCommit)
}

// TestDo_RetriesTransientErrors checks that a serialization failure is
// retried until it succeeds.
func TestDo_RetriesTransientErrors(t *testing.T) {
	tx := &pgxmocks.Tx{}
	tx.On("Commit", mock.Anything).Return(nil).Once()
	tx.On("Rollback", mock.Anything).Return(nil).Twice()

	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, mock.Anything).Return(tx, nil).Times(3)

	m := database.NewTxManager(pool, database.WithMaxRetries(3), database.WithRetryDelay(time.Millisecond))

	attempts := 0
	err := m.Do(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return serializationError()
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

// TestDo_ExhaustsRetries checks that the retry budget is respected.
func TestDo_ExhaustsRetries(t *testing.T) {
	tx := &pgxmocks.Tx{}
	tx.On("Commit", mock.Anything).Return(nil).Maybe()
	tx.On("Rollback", mock.Anything).Return(nil).Times(3)

	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, mock.Anything).Return(tx, nil).Times(3)

	m := database.NewTxManager(pool, database.WithMaxRetries(2), database.WithRetryDelay(time.Millisecond))

	attempts := 0
	err := m.Do(context.Background(), func(context.Context) error {
		attempts++
		return serializationError()
	})

	require.Error(t, err)
	require.Equal(t, 3, attempts, "the first attempt plus two retries")
}

// serializationError returns a pgconn error with the serialization failure
// code, so the tx manager treats it as transient.
func serializationError() error {
	return &pgconn.PgError{Code: "40001", Message: "serialization failure"}
}
