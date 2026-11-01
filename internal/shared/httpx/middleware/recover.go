package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
)

// NewRecover creates a middleware that catches any panics during request handling.
// It logs the stack trace and returns a graceful 500 Internal Server Error response to the client.
func NewRecover(log *slog.Logger) fiber.Handler {
	if log == nil {
		log = slog.Default()
	}

	return func(c fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					"method", c.Method(),
					"path", c.Path(),
					"panic", r,
					"stack", string(debug.Stack()),
				)

				_ = response.WriteProblem(c, &response.Problem{
					Title:  "Internal Server Error",
					Status: http.StatusInternalServerError,
					Detail: "An internal server error has occurred. Our team has logged this issue.",
				})
			}
		}()

		return c.Next()
	}
}
