package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
)

// NewErrorHandler catches all errors returned by the handler
func NewErrorHandler(log *slog.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		// If the error is already in the *Problem struct format (RFC 7807)
		if prob, ok := response.From(err); ok {
			return response.WriteProblem(c, prob)
		}

		// If the error is a built-in Fiber error (e.g., 404 Route Not Found, 405 Method Not Allowed)
		if fe, ok := errors.AsType[*fiber.Error](err); ok {
			return response.WriteProblem(c, &response.Problem{
				Title:  http.StatusText(fe.Code),
				Status: fe.Code,
				Detail: fe.Message,
			})
		}

		// Default catch-all for unexpected errors (500).
		// The real error is logged only; never echo internals (SQL, paths) to the client.
		log.Error("http: unexpected error", "method", c.Method(), "path", c.Path(), "err", err)
		return response.WriteProblem(c, &response.Problem{
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Detail: "An unexpected error occurred.",
		})
	}
}
