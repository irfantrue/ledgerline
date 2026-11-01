package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// txKey is the context key for storing database transactions.
type txKey struct{}

// maxBackoffShift caps the exponential backoff between retries. A shift of 63
// or more overflows the int64 and turns the delay negative, which would fire
// the timer immediately instead of backing off.
const maxBackoffShift = 10

// DBTX is satisfied by both *pgxpool.Pool and pgx.Tx. It matches the
// interface sqlc generates for pgx/v5, so repositories can use it directly.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// IsolationLevel maps a string to a pgx.TxIsoLevel.
// Default is ReadCommitted if the string is not recognized.
func IsolationLevel(s string) pgx.TxIsoLevel {
	switch s {
	case "serializable":
		return pgx.Serializable
	case "repeatable read":
		return pgx.RepeatableRead
	case "read committed":
		return pgx.ReadCommitted
	case "read uncommitted":
		return pgx.ReadUncommitted
	default:
		return pgx.ReadCommitted
	}
}

// TxManager manages database transactions with automatic commit/rollback handling.
type TxManager struct {
	pool           *pgxpool.Pool
	isolationLevel pgx.TxIsoLevel
	maxRetries     int
	retryDelay     time.Duration
}

// TxManagerOption configures a TxManager.
type TxManagerOption func(*TxManager)

// WithIsolationLevel sets the transaction isolation level.
// Default: pgx.ReadCommitted.
func WithIsolationLevel(level pgx.TxIsoLevel) TxManagerOption {
	return func(m *TxManager) { m.isolationLevel = level }
}

// WithMaxRetries sets the maximum number of retries for transient errors
// (deadlock, serialization failure). Default: 3.
func WithMaxRetries(n int) TxManagerOption {
	return func(m *TxManager) { m.maxRetries = n }
}

// WithRetryDelay sets the base delay between retries. Default: 100ms.
// Exponential backoff is applied: delay *= 2^(attempt-1).
func WithRetryDelay(d time.Duration) TxManagerOption {
	return func(m *TxManager) { m.retryDelay = d }
}

// NewTxManager creates a new TxManager with the given connection pool and options.
func NewTxManager(pool *pgxpool.Pool, opts ...TxManagerOption) *TxManager {
	m := &TxManager{
		pool:           pool,
		isolationLevel: pgx.ReadCommitted,
		maxRetries:     3,
		retryDelay:     100 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// TxFrom retrieves an existing transaction from the context, if any.
func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// ContextWithTx returns a copy of ctx carrying the given transaction.
// TxFrom and TxManager.Querier pick it up from there.
func ContextWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// Querier returns the transaction in ctx when there is one, otherwise the pool.
// Repositories should call this so they join the caller's transaction automatically.
func (m *TxManager) Querier(ctx context.Context) DBTX {
	if tx, ok := TxFrom(ctx); ok {
		return tx
	}
	return m.pool
}

// isTransientError returns true if the error is a deadlock or serialization failure
// that can be retried.
func isTransientError(err error) bool {
	if pgErr, _ := errors.AsType[*pgconn.PgError](err); pgErr != nil {
		// 40P01: deadlock_detected
		// 40001: serialization_failure
		return pgErr.Code == "40P01" || pgErr.Code == "40001"
	}
	return false
}

// Do executes the given function within a transaction.
// If a transaction already exists in the context, it reuses it (no nesting).
// Otherwise, it creates a new transaction, committing on success or rolling back on failure.
// Transient errors (deadlocks, serialization failures) are retried up to maxRetries times.
//
// The function is called again on every retry, so it has to be idempotent and
// must not have side effects outside the database: a publish to the broker or
// a cache write would happen once per attempt.
func (m *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := TxFrom(ctx); ok {
		return fn(ctx)
	}

	var attempt int
	for attempt = 0; attempt <= m.maxRetries; attempt++ {
		if attempt > 0 {
			// The exponent is capped: a shift of 63 or more overflows the
			// int64 and turns the delay negative, which would fire the timer
			// immediately instead of backing off.
			shift := min(attempt-1, maxBackoffShift)
			delay := m.retryDelay * time.Duration(1<<shift)
			slog.Info("[TX] retrying transaction after transient error", "attempt", attempt, "delay", delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err = m.doOnce(ctx, fn)
		if err == nil {
			return nil
		}

		if !isTransientError(err) {
			return err
		}

		slog.Warn("[TX] transient error, will retry", "attempt", attempt+1, "error", err)
	}

	return fmt.Errorf("txManager.Do: exceeded max retries (%d): %w", m.maxRetries, err)
}

// doOnce runs fn inside a single transaction attempt.
func (m *TxManager) doOnce(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: m.isolationLevel})
	if err != nil {
		return err
	}

	defer func() {
		// Rollback must still work if the request ctx was cancelled/timed out.
		rbCtx := context.WithoutCancel(ctx)
		if p := recover(); p != nil {
			_ = tx.Rollback(rbCtx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(rbCtx)
		}
	}()

	if err = fn(ContextWithTx(ctx, tx)); err != nil {
		return err
	}

	// The commit error has to land in err: the deferred rollback reads it, and
	// a commit is exactly where a deferred constraint (a journal that does not
	// balance) is rejected.
	err = tx.Commit(ctx)
	return err
}
