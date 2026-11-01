package databasetest

import (
	"context"
	"os"
	"testing"
	"time"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIsolationLevel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want pgx.TxIsoLevel
	}{
		{name: "serializable", in: "serializable", want: pgx.Serializable},
		{name: "repeatable read", in: "repeatable read", want: pgx.RepeatableRead},
		{name: "read committed", in: "read committed", want: pgx.ReadCommitted},
		{name: "read uncommitted", in: "read uncommitted", want: pgx.ReadUncommitted},
		{name: "unknown falls back", in: "nonsense", want: pgx.ReadCommitted},
		{name: "empty falls back", in: "", want: pgx.ReadCommitted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := database.IsolationLevel(tt.in)
			if got != tt.want {
				t.Errorf("IsolationLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewTxManager_Options(t *testing.T) {
	m := database.NewTxManager(nil,
		database.WithIsolationLevel(database.IsolationLevel("serializable")),
		database.WithMaxRetries(5),
		database.WithRetryDelay(10*time.Millisecond),
	)

	if m == nil {
		t.Fatal("NewTxManager() = nil")
	}
}

func TestNewPostgres(t *testing.T) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN not set; skipping integration test")
	}

	pool, err := database.NewPostgres(context.Background(), config.Config{
		DB: config.DatabaseConfig{
			DSN:               dsn,
			MaxOpenConns:      5,
			MinOpenConns:      1,
			MaxConnLifetime:   time.Hour,
			MaxConnIdleTime:   time.Minute,
			HealthCheckPeriod: time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("NewPostgres() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

// TestNewPostgres_InvalidDSN checks the error path of the configuration
// parser.
func TestNewPostgres_InvalidDSN(t *testing.T) {
	_, err := database.NewPostgres(context.Background(), config.Config{
		DB: config.DatabaseConfig{DSN: "not a dsn"},
	})
	if err == nil {
		t.Fatal("NewPostgres() error = nil, want a parse error")
	}
}

// TestNewPostgres_UnreachableHost checks that a pool that cannot be reached
// is reported instead of being handed out.
func TestNewPostgres_UnreachableHost(t *testing.T) {
	//nolint:gosec // G101: a test DSN pointing at a closed port, not a credential
	_, err := database.NewPostgres(context.Background(), config.Config{
		DB: config.DatabaseConfig{DSN: "postgres://postgres:postgres@127.0.0.1:1/app?sslmode=disable"},
	})
	if err == nil {
		t.Fatal("NewPostgres() error = nil, want a connection error")
	}
}

func TestNewRedisClient(t *testing.T) {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		t.Skip("REDIS_HOST not set; skipping integration test")
	}

	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}

	client, err := database.NewRedisClient(context.Background(), config.Config{
		Redis: config.RedisConfig{Host: host, Port: port, Password: os.Getenv("REDIS_PASSWORD")},
	})
	if err != nil {
		t.Fatalf("NewRedisClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

// TestNewRedisClient_UnreachableHost checks the error path.
func TestNewRedisClient_UnreachableHost(t *testing.T) {
	_, err := database.NewRedisClient(context.Background(), config.Config{
		Redis: config.RedisConfig{Host: "127.0.0.1", Port: "1"},
	})
	if err == nil {
		t.Fatal("NewRedisClient() error = nil, want a connection error")
	}
}

// TestDo_RetriesTransientErrors checks that a serialization failure is
// retried until it succeeds.
func TestDo_RetriesTransientErrors(t *testing.T) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN not set; skipping integration test")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	m := database.NewTxManager(pool,
		database.WithMaxRetries(3),
		database.WithRetryDelay(time.Millisecond),
	)

	attempts := 0
	err = m.Do(context.Background(), func(_ context.Context) error {
		attempts++
		if attempts < 3 {
			return serializationError()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

// TestDo_ExhaustsRetries checks that the retry budget is respected.
func TestDo_ExhaustsRetries(t *testing.T) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		t.Skip("DB_DSN not set; skipping integration test")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	m := database.NewTxManager(pool,
		database.WithMaxRetries(2),
		database.WithRetryDelay(time.Millisecond),
	)

	attempts := 0
	err = m.Do(context.Background(), func(context.Context) error {
		attempts++
		return serializationError()
	})
	if err == nil {
		t.Fatal("Do() error = nil, want the retries to be exhausted")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (the first try plus two retries)", attempts)
	}
}

// serializationError returns a pgconn error with the serialization failure
// code, so the tx manager treats it as transient.
func serializationError() error {
	return &pgconn.PgError{Code: "40001", Message: "serialization failure"}
}
