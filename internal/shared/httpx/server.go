package httpx

import (
	"log/slog"
	"time"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/httpx/middleware"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
)

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 15 * time.Second
	idleTimeout  = 60 * time.Second
	bodyLimit    = 1 * 1024 * 1024 // 1 MiB
)

// NewFiber creates and returns a new Fiber application instance
// with the default configuration.
func NewFiber(conf config.Config, logger *slog.Logger) *fiber.App {
	if logger == nil {
		logger = slog.Default()
	}
	return fiber.New(fiber.Config{
		AppName:      conf.App.AppName,
		ServerHeader: conf.App.ServerHeader,
		JSONEncoder:  sonic.Marshal,
		JSONDecoder:  sonic.Unmarshal,
		ErrorHandler: middleware.NewErrorHandler(logger),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
		BodyLimit:    bodyLimit,
	})
}
