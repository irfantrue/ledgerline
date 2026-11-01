package middleware

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// RequestLogger returns a Fiber middleware that logs HTTP requests with structured logging.
// Note: Ensure the requestid middleware is registered before this middleware for request_id to be populated.
//
// Response body is NOT logged to avoid leaking sensitive data (balances, tokens, PII).
func RequestLogger(logger *slog.Logger) fiber.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		// Determine status code: prefer fiber.Error code over response status
		// since handlers may set response status before returning an error with a different code.
		status := c.Response().StatusCode()
		var errMsg string
		if err != nil {
			errMsg = err.Error()
			if fe, _ := errors.AsType[*fiber.Error](err); fe != nil {
				status = fe.Code
			}
		}

		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}

		// Log response metadata only - never log body to avoid leaking sensitive data.
		attrs := []slog.Attr{
			// The request id lives in the Fiber locals, so the Ctx has to be
			// passed here. Passing c.Context() would always yield an empty id.
			slog.String("request_id", requestid.FromContext(c)),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.String("route", c.Route().Path),
			slog.Int("status", status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("client_ip", c.IP()),
			slog.String("user_agent", c.Get(fiber.HeaderUserAgent)),
			slog.Int64("resp_size", int64(c.Response().Header.ContentLength())),
		}
		if errMsg != "" {
			attrs = append(attrs, slog.String("error", errMsg))
		}

		logger.LogAttrs(c.Context(), level, "http_request", attrs...)
		return err
	}
}
