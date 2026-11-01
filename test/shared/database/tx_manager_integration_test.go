package databasetest

import (
	"context"
	"errors"
	"os"
	"testing"

	"ledgerline/internal/shared/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests need a real PostgreSQL. Point DB_DSN at a throwaway
// database (the test creates and drops its own table), e.g.:
//
//	DB_DSN='postgres://postgres:postgres@localhost:5432/ledgerline_test?sslmode=disable' go test ./test/shared/database/...
//
// Without it these tests are skipped, so plain `make test` stays DB-free.

var errBoom = errors.New("boom")

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_manager_it (id BIGSERIAL PRIMARY KEY, note TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE tx_manager_it`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Cleanups run LIFO: drop first, then pool.Close.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS tx_manager_it`)
	})

	return pool
}

func insertNote(ctx context.Context, q database.DBTX, note string) error {
	_, err := q.Exec(ctx, `INSERT INTO tx_manager_it (note) VALUES ($1)`, note)
	return err
}

func countNotes(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM tx_manager_it`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestIntegration_Do_CommitsOnSuccess(t *testing.T) {
	pool := testPool(t)
	m := database.NewTxManager(pool)

	err := m.Do(context.Background(), func(ctx context.Context) error {
		if err := insertNote(ctx, m.Querier(ctx), "a"); err != nil {
			return err
		}
		// Read Committed: other connections must not see the uncommitted row.
		if n := countNotes(t, pool); n != 0 {
			t.Errorf("uncommitted row visible outside tx: %d", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	if n := countNotes(t, pool); n != 1 {
		t.Fatalf("rows after commit = %d, want 1", n)
	}
}

func TestIntegration_Do_RollsBackOnError(t *testing.T) {
	pool := testPool(t)
	m := database.NewTxManager(pool)

	err := m.Do(context.Background(), func(ctx context.Context) error {
		if err := insertNote(ctx, m.Querier(ctx), "a"); err != nil {
			return err
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}

	if n := countNotes(t, pool); n != 0 {
		t.Fatalf("rows after rollback = %d, want 0", n)
	}
}

func TestIntegration_Do_RollsBackAndRepanics(t *testing.T) {
	pool := testPool(t)
	m := database.NewTxManager(pool)

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("panic should propagate after rollback")
			}
		}()
		_ = m.Do(context.Background(), func(ctx context.Context) error {
			_ = insertNote(ctx, m.Querier(ctx), "a")
			panic("kaboom")
		})
	}()

	if n := countNotes(t, pool); n != 0 {
		t.Fatalf("rows after panic = %d, want 0", n)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("leaked %d connection(s) after panic", got)
	}
}

// Regression: nested Do must join the outer tx, so an outer failure undoes inner work.
func TestIntegration_Do_NestedJoinsOuterTx(t *testing.T) {
	pool := testPool(t)
	m := database.NewTxManager(pool)

	err := m.Do(context.Background(), func(ctx context.Context) error {
		if err := insertNote(ctx, m.Querier(ctx), "outer"); err != nil {
			return err
		}
		if err := m.Do(ctx, func(ctx context.Context) error {
			return insertNote(ctx, m.Querier(ctx), "inner")
		}); err != nil {
			return err
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}

	if n := countNotes(t, pool); n != 0 {
		t.Fatalf("rows = %d, want 0 (inner work must roll back with outer)", n)
	}
}

// Regression: rollback used the request ctx, which fails if that ctx is cancelled.
func TestIntegration_Do_RollbackSurvivesCancelledContext(t *testing.T) {
	pool := testPool(t)
	m := database.NewTxManager(pool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := m.Do(ctx, func(ctx context.Context) error {
		if err := insertNote(ctx, m.Querier(ctx), "a"); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	if n := countNotes(t, pool); n != 0 {
		t.Fatalf("rows = %d, want 0", n)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("leaked %d connection(s)", got)
	}
}

// TestIntegration_Do_ReportsCommitFailure covers the path where the work
// itself succeeds but the commit fails: a deferred constraint is only checked
// at COMMIT. The error has to reach the caller and the transaction must not
// leave a connection behind.
func TestIntegration_Do_ReportsCommitFailure(t *testing.T) {
	pool := testPool(t)
	m := database.NewTxManager(pool)

	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS tx_manager_deferred (
			id BIGSERIAL PRIMARY KEY,
			note TEXT NOT NULL,
			CONSTRAINT note_unique UNIQUE (note) DEFERRABLE INITIALLY DEFERRED
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS tx_manager_deferred`)
	})

	if _, err := pool.Exec(context.Background(), `INSERT INTO tx_manager_deferred (note) VALUES ('boom')`); err != nil {
		t.Fatalf("insert first row: %v", err)
	}

	// The duplicate is only detected at COMMIT, because the unique constraint
	// is deferred.
	err := m.Do(context.Background(), func(ctx context.Context) error {
		_, err := m.Querier(ctx).Exec(ctx, `INSERT INTO tx_manager_deferred (note) VALUES ('boom')`)
		return err
	})
	if err == nil {
		t.Fatal("Do() error = nil, want the deferred unique violation")
	}

	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("leaked %d connection(s) after a failed commit", got)
	}
}

func TestIntegration_Querier(t *testing.T) {
	pool := testPool(t)
	m := database.NewTxManager(pool)

	if _, ok := m.Querier(context.Background()).(*pgxpool.Pool); !ok {
		t.Error("outside a tx, Querier should return the pool")
	}

	_ = m.Do(context.Background(), func(ctx context.Context) error {
		if _, ok := m.Querier(ctx).(pgx.Tx); !ok {
			t.Error("inside Do, Querier should return the tx")
		}
		return nil
	})
}
