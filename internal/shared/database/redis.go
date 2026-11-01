package database

import (
	"context"
	"fmt"
	"log/slog"

	"ledgerline/internal/shared/config"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates and pings a Redis client connection.
// It logs the successful connection using the global slogger.
func NewRedisClient(ctx context.Context, conf config.Config) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%s", conf.Redis.Host, conf.Redis.Port)

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: conf.Redis.Password,
		DB:       conf.Redis.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	slog.Info("successfully connected to the Redis database.")
	return client, nil
}
