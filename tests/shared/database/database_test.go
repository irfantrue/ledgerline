package databasetest

import (
	"context"
	"testing"
	"time"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/database"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
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
			require.Equal(t, tt.want, database.IsolationLevel(tt.in))
		})
	}
}

func TestNewTxManager_Options(t *testing.T) {
	m := database.NewTxManager(nil,
		database.WithIsolationLevel(database.IsolationLevel("serializable")),
		database.WithMaxRetries(5),
		database.WithRetryDelay(10*time.Millisecond),
	)

	require.NotNil(t, m)
}

// TestNewPostgres_InvalidDSN checks the error path of the configuration
// parser. The pool is never opened, so no database is needed.
func TestNewPostgres_InvalidDSN(t *testing.T) {
	_, err := database.NewPostgres(context.Background(), config.Config{
		DB: config.DatabaseConfig{DSN: "not a dsn"},
	})

	require.Error(t, err, "a DSN that is not a DSN should be rejected")
}

// TestNewPostgres_UnreachableHost checks that a pool that cannot be reached
// is reported instead of being handed out. The address points at a closed
// port on the loopback interface, so no database is needed.
func TestNewPostgres_UnreachableHost(t *testing.T) {
	//nolint:gosec // G101: a test DSN pointing at a closed port, not a credential
	_, err := database.NewPostgres(context.Background(), config.Config{
		DB: config.DatabaseConfig{DSN: "postgres://postgres:postgres@127.0.0.1:1/app?sslmode=disable"},
	})

	require.Error(t, err, "a pool that cannot be reached should be reported")
}

// TestNewRedisClient_UnreachableHost checks the error path. The address points
// at a closed port on the loopback interface, so no Redis is needed.
func TestNewRedisClient_UnreachableHost(t *testing.T) {
	_, err := database.NewRedisClient(context.Background(), config.Config{
		Redis: config.RedisConfig{Host: "127.0.0.1", Port: "1"},
	})

	require.Error(t, err, "a Redis that cannot be reached should be reported")
}
