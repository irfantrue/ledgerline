package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/httpx/middleware"
	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
)

const readyTimeout = 2 * time.Second

// Pinger is satisfied by *pgxpool.Pool; used by the readiness probe.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Config holds the dependencies required to set up HTTP routes.
type Config struct {
	App    *fiber.App
	Config config.Config
	Logger *slog.Logger
	DB     Pinger // optional
}

// RegisterRoutes configures the global middlewares and registers all API endpoints.
func RegisterRoutes(conf *Config) {
	// 1. RECOVER (Must always be number 1 to catch panics)
	conf.App.Use(middleware.NewRecover(conf.Logger))

	// 2. CORS (Placed second so the browser obtains cross-domain permission quickly)
	conf.App.Use(middleware.NewCors(conf.Config))

	// 3. HELMET (Third in the sequence for injecting Security Headers)
	conf.App.Use(middleware.NewHelmet())

	// 4. Other Global Middleware
	conf.App.Use(middleware.NewCompress(conf.Config))
	conf.App.Use(middleware.NewTimezone(conf.Config.App.Timezone))
	conf.App.Use(middleware.NewRequestID())
	conf.App.Use(middleware.RequestLogger(conf.Logger))

	// Liveness: process is up.
	conf.App.Get("/healthz", func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[any]{
			Status: fiber.StatusOK,
			Data: fiber.Map{
				"status": "active",
			},
		})
	})

	// Readiness: dependencies are reachable.
	conf.App.Get("/readyz", func(c fiber.Ctx) error {
		if conf.DB != nil {
			ctx, cancel := context.WithTimeout(c, readyTimeout)
			defer cancel()
			if err := conf.DB.Ping(ctx); err != nil {
				return fiber.NewError(http.StatusServiceUnavailable, "database unavailable")
			}
		}
		return response.WriteSuccess(c, &response.Response[any]{
			Status: fiber.StatusOK,
			Data: fiber.Map{
				"database": "ready",
			},
		})
	})
}
