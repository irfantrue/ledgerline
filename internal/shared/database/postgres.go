package database

import (
	"context"
	"log/slog"
	"time"

	"ledgerline/internal/shared/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Default connection and health check timeout for database connections.
const dbTimeout = 10 * time.Second

// NewPostgres creates a new PostgreSQL connection pool using the provided configuration.
// It validates the connection by pinging the database and returns the pool or an error.
// Zero-valued pool settings keep pgxpool's defaults instead of overriding them
// (a zero HealthCheckPeriod or MaxConns would otherwise break the pool).
func NewPostgres(ctx context.Context, conf config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(conf.DB.DSN)
	if err != nil {
		return nil, err
	}

	if conf.DB.MaxOpenConns > 0 {
		poolConfig.MaxConns = conf.DB.MaxOpenConns
	}
	if conf.DB.MinOpenConns > 0 {
		poolConfig.MinConns = conf.DB.MinOpenConns
	}
	if conf.DB.MaxConnLifetime > 0 {
		poolConfig.MaxConnLifetime = conf.DB.MaxConnLifetime
	}
	if conf.DB.MaxConnIdleTime > 0 {
		poolConfig.MaxConnIdleTime = conf.DB.MaxConnIdleTime
	}
	if conf.DB.HealthCheckPeriod > 0 {
		poolConfig.HealthCheckPeriod = conf.DB.HealthCheckPeriod
	}

	connectCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, err
	}

	slog.Info("successfully connected to the Postgres database.")
	return pool, nil
}
