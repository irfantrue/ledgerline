package middlewaretest

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/httpx/middleware"
	"ledgerline/internal/shared/httpx/response"
	"ledgerline/pkg/ctxval"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tzJakarta is the timezone used across the middleware tests.
const tzJakarta = "Asia/Jakarta"

// ctxvalTimezone returns the timezone the middleware stored in the context.
func ctxvalTimezone(c fiber.Ctx) string {
	return ctxval.GetTimezone(c.Context()).String()
}

func TestNewCors(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewCors(config.Config{
		App: config.AppConfig{CorsAllowedOrigins: "https://a.example.com, https://b.example.com"},
	}))
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := request(t, "/")
	req.Header.Set("Origin", "https://a.example.com")

	res := do(t, app, req)

	assert.Equal(t, "https://a.example.com", res.Header.Get("Access-Control-Allow-Origin"), "Allow-Origin")
}

func TestNewCors_NoOrigins(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewCors(config.Config{}))
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := request(t, "/")
	req.Header.Set("Origin", "https://a.example.com")

	res := do(t, app, req)

	assert.NotEqual(t, "https://a.example.com", res.Header.Get("Access-Control-Allow-Origin"),
		"Allow-Origin should not be echoed when no origins are configured")
}

func TestNewHelmet(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewHelmet())
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	res := do(t, app, request(t, "/"))

	assert.NotEmpty(t, res.Header.Get("X-XSS-Protection"), "X-XSS-Protection header is missing")
}

func TestNewCompress(t *testing.T) {
	tests := []struct {
		name  string
		level int
	}{
		{name: "disabled", level: 0},
		{name: "best speed", level: 1},
		{name: "best compression", level: 2},
		{name: "unknown falls back", level: 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(middleware.NewCompress(config.Config{
				App: config.AppConfig{CompressLevel: tt.level},
			}))
			app.Get("/", func(c fiber.Ctx) error {
				return c.SendString(strings.Repeat("compressible payload ", 100))
			})

			req := request(t, "/")
			req.Header.Set("Accept-Encoding", "gzip")

			res := do(t, app, req)

			require.Equal(t, http.StatusOK, res.Status, "status")

			if tt.level == 0 {
				assert.Empty(t, res.Header.Get("Content-Encoding"), "Content-Encoding, want no compression")
				return
			}
			assert.Equal(t, "gzip", res.Header.Get("Content-Encoding"), "Content-Encoding")
			assert.NotEmpty(t, res.Body, "body is empty")
		})
	}
}

func TestNewRequestID(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewRequestID())
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(requestid.FromContext(c))
	})

	res := do(t, app, request(t, "/"))

	assert.NotEmpty(t, res.Header.Get("X-Request-ID"), "X-Request-ID header is missing")
	assert.NotEmpty(t, res.Body, "the request id was not stored in the context")
	assert.Equal(t, res.Header.Get("X-Request-ID"), res.Body, "the context id should match the header")
}

func TestRequestLogger(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewRequestID())
	app.Use(middleware.RequestLogger(slog.Default()))
	app.Get("/ok", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	res := do(t, app, request(t, "/ok"))

	assert.Equal(t, http.StatusOK, res.Status, "status")
}

// TestRequestLogger_ErrorStatus checks that an error from the handler is
// logged and passed through.
func TestRequestLogger_ErrorStatus(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewRequestID())
	app.Use(middleware.RequestLogger(slog.Default()))
	app.Get("/boom", func(_ fiber.Ctx) error {
		return fiber.NewError(http.StatusInternalServerError, "boom")
	})

	res := do(t, app, request(t, "/boom"))

	assert.Equal(t, http.StatusInternalServerError, res.Status, "status")
}

// TestRequestLogger_NilLogger makes sure the middleware does not panic when it
// is handed no logger.
func TestRequestLogger_NilLogger(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RequestLogger(nil))
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	res := do(t, app, request(t, "/"))

	assert.Equal(t, http.StatusOK, res.Status, "status")
}

func TestNewTimezone_Header(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewTimezone(tzJakarta))
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(ctxvalTimezone(c))
	})

	req := request(t, "/")
	req.Header.Set("X-Timezone", "Europe/Berlin")

	res := do(t, app, req)

	assert.Equal(t, "Europe/Berlin", res.Body, "timezone")
}

func TestNewTimezone_InvalidHeaderFallsBackToDefault(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewTimezone(tzJakarta))
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(ctxvalTimezone(c))
	})

	req := request(t, "/")
	req.Header.Set("X-Timezone", "Not/AZone")

	res := do(t, app, req)

	assert.Equal(t, tzJakarta, res.Body, "timezone")
}

func TestNewTimezone_InvalidDefaultFallsBackToUTC(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewTimezone("Not/AZone"))
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(ctxvalTimezone(c))
	})

	res := do(t, app, request(t, "/"))

	assert.Equal(t, "UTC", res.Body, "timezone")
}

func TestNewErrorHandler(t *testing.T) {
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.NewErrorHandler(slog.Default()),
	})
	app.Get("/boom", func(_ fiber.Ctx) error {
		return fiber.NewError(http.StatusTeapot, "boom")
	})

	res := do(t, app, request(t, "/boom"))

	assert.Equal(t, http.StatusTeapot, res.Status, "status")
	assert.Contains(t, res.Body, "boom", "the body should contain the error detail")
}

// TestNewErrorHandler_UnexpectedError checks that an unknown error becomes a
// 500 without leaking internals.
func TestNewErrorHandler_UnexpectedError(t *testing.T) {
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.NewErrorHandler(slog.Default()),
	})
	app.Get("/boom", func(_ fiber.Ctx) error {
		return errors.New("database password is hunter2")
	})

	res := do(t, app, request(t, "/boom"))

	assert.Equal(t, http.StatusInternalServerError, res.Status, "status")
	assert.NotContains(t, res.Body, "hunter2", "the body should not leak the internal error")
}

func TestBadRequestResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return response.BadRequestResponse(c, "Invalid payload", "the body is malformed", []response.InvalidParam{
			{Name: "name", Reason: "is required"},
		})
	})

	res := do(t, app, request(t, "/"))

	assert.Equal(t, http.StatusBadRequest, res.Status, "status")
	assert.Contains(t, res.Body, "is required", "the body should contain the invalid parameter")
}

func TestUnprocessableEntityResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return response.UnprocessableEntityResponse(c, "Unprocessable", "the balance does not add up")
	})

	res := do(t, app, request(t, "/"))

	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "status")
	assert.Contains(t, res.Body, "does not add up", "the body should contain the detail")
}
