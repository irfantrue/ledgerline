// Package main is the entry point of the application.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ledgerline/internal/shared/broker"
	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/database"
	"ledgerline/internal/shared/httpx"
	"ledgerline/internal/shared/logger"

	"github.com/gofiber/fiber/v3"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run wires dependencies and blocks until shutdown. Returning an error
// (instead of calling os.Exit inline) lets deferred cleanups run.
func run() error {
	// Set up signal handling for graceful shutdown (SIGINT/SIGTERM)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conf, err := config.LoadConfig(".env")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(conf.Log.Level, conf.App.Env)
	slog.SetDefault(log)

	pool, err := database.NewPostgres(ctx, conf)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pool.Close()

	cache, err := database.NewRedisClient(ctx, conf)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	rabbitMQ, err := broker.NewRabbitMQ(conf)
	if err != nil {
		return fmt.Errorf("rabbitmq: %w", err)
	}

	app := httpx.NewFiber(conf, log)
	httpx.RegisterRoutes(&httpx.Config{
		App:    app,
		Config: conf,
		Logger: log,
		DB:     pool,
	})

	if err := listen(ctx, app, log, conf.App.Port); err != nil {
		return err
	}

	return shutdown(app, cache, rabbitMQ, log)
}

// listen starts the HTTP server and blocks until it fails or the context is
// cancelled by a termination signal.
func listen(ctx context.Context, app *fiber.App, log *slog.Logger, port string) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("server is running", "port", port)
		if err := app.Listen(":" + port); err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
		log.Info("received termination signal, initiating graceful shutdown")
	}

	return nil
}

// shutdown drains the HTTP server and closes the connections that were opened
// during startup. The closers are only used for their Close method, so they
// are taken as io.Closer.
func shutdown(app *fiber.App, cache io.Closer, rabbitMQ io.Closer, log *slog.Logger) error {
	// Allow time to drain active requests. Fresh context: ctx is already cancelled.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Error("fiber shutdown failed", "err", err)
	}

	if err := cache.Close(); err != nil {
		return fmt.Errorf("redis close: %w", err)
	}

	if err := rabbitMQ.Close(); err != nil {
		return fmt.Errorf("rabbitmq close: %w", err)
	}

	log.Info("graceful shutdown complete")
	return nil
}
