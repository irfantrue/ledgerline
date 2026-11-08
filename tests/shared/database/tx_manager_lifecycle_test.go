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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestDo_RollsBackWhenTheFunctionPanics checks that a panic inside the
// transaction function does not leave the transaction open: the manager rolls
// it back and lets the panic continue.
func TestDo_RollsBackWhenTheFunctionPanics(t *testing.T) {
	tx := &pgxmocks.Tx{}
	tx.On("Rollback", mock.Anything).Return(nil).Once()

	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, mock.Anything).Return(tx, nil).Once()

	m := database.NewTxManager(pool)

	require.PanicsWithValue(t, "boom", func() {
		_ = m.Do(context.Background(), func(context.Context) error {
			panic("boom")
		})
	})

	tx.AssertExpectations(t)
}

// TestDo_StopsWhenTheContextIsCancelled checks that a cancelled context
// ends the transaction instead of starting another attempt.
func TestDo_StopsWhenTheContextIsCancelled(t *testing.T) {
	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, mock.Anything).Return(nil, context.Canceled).Once()

	m := database.NewTxManager(pool, database.WithMaxRetries(5), database.WithRetryDelay(50*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Do(ctx, func(context.Context) error {
		return errors.New("the function must not run")
	})

	require.ErrorIs(t, err, context.Canceled)
	pool.AssertExpectations(t)
}

// TestQuerier_ReturnsThePoolWithoutATransaction checks that a call outside a
// transaction gets the pool, so it joins the transaction the caller opens
// around it.
func TestQuerier_ReturnsThePoolWithoutATransaction(t *testing.T) {
	pool := &databasemocks.Pool{}
	m := database.NewTxManager(pool)

	require.Same(t, database.DBTX(pool), m.Querier(context.Background()))
}

// TestDo_UsesTheIsolationLevelOfTheManager checks that the manager hands the
// configured isolation level to the pool.
func TestDo_UsesTheIsolationLevelOfTheManager(t *testing.T) {
	tx := &pgxmocks.Tx{}
	tx.On("Commit", mock.Anything).Return(nil).Once()

	pool := &databasemocks.Pool{}
	pool.On("BeginTx", mock.Anything, pgx.TxOptions{IsoLevel: pgx.Serializable}).
		Return(tx, nil).
		Once()

	m := database.NewTxManager(pool, database.WithIsolationLevel(database.IsolationLevel("serializable")))

	require.NoError(t, m.Do(context.Background(), func(context.Context) error { return nil }))

	pool.AssertExpectations(t)
}
