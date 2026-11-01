package middleware

import (
	"ledgerline/internal/shared/config"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
)

// NewCompress creates a new Fiber compression middleware based on the application configuration.
// The compression level is determined by the CompressLevel value in the config.
func NewCompress(cfg config.Config) fiber.Handler {
	level := compress.LevelBestSpeed

	switch cfg.App.CompressLevel {
	case 0:
		level = compress.LevelDisabled
	case 1:
		level = compress.LevelBestSpeed
	case 2:
		level = compress.LevelBestCompression
	}

	return compress.New(compress.Config{
		Level: level,
	})
}
