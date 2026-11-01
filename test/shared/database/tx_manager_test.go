package databasetest

import (
	"context"
	"errors"
	"testing"

	"ledgerline/internal/shared/database"

	"github.com/jackc/pgx/v5"
)

// fakeTx satisfies pgx.Tx (and DBTX) via embedding. Calling any method panics,
// which is exactly what we want: these tests assert the code never touches it.
type fakeTx struct{ pgx.Tx }

func ctxWithTx(tx pgx.Tx) context.Context {
	return database.ContextWithTx(context.Background(), tx)
}

func TestTxFrom_NoTx(t *testing.T) {
	if _, ok := database.TxFrom(context.Background()); ok {
		t.Fatal("TxFrom on empty context should return false")
	}
}

// Regression: TxFrom used to assert pgxpool.Tx, but the stored value is a
// pgx.Tx, so the assertion always failed and transactions were never reused.
func TestTxFrom_ReturnsStoredTx(t *testing.T) {
	want := &fakeTx{}

	got, ok := database.TxFrom(ctxWithTx(want))
	if !ok {
		t.Fatal("TxFrom should find the stored pgx.Tx")
	}
	if got != pgx.Tx(want) {
		t.Fatal("TxFrom returned a different tx")
	}
}

func TestDo_ReusesExistingTx(t *testing.T) {
	want := &fakeTx{}
	m := database.NewTxManager(nil) // nil pool: must not be used when a tx already exists

	called := false
	err := m.Do(ctxWithTx(want), func(ctx context.Context) error {
		called = true
		got, ok := database.TxFrom(ctx)
		if !ok || got != pgx.Tx(want) {
			t.Error("inner ctx should carry the same tx")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !called {
		t.Fatal("fn was not called")
	}
}

func TestDo_ReusedTxPropagatesErrorWithoutRollback(t *testing.T) {
	errBoom := errors.New("boom")
	m := database.NewTxManager(nil)

	// fakeTx panics on Rollback/Commit, so passing means the outer owner decides.
	err := m.Do(ctxWithTx(&fakeTx{}), func(context.Context) error { return errBoom })
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestQuerier_PrefersTx(t *testing.T) {
	tx := &fakeTx{}
	m := database.NewTxManager(nil)

	if q := m.Querier(ctxWithTx(tx)); q != database.DBTX(tx) {
		t.Fatal("Querier should return the tx from ctx")
	}
}
