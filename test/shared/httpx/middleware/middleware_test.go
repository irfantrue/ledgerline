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

	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://a.example.com" {
		t.Errorf("Allow-Origin = %q, want %q", got, "https://a.example.com")
	}
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

	if got := res.Header.Get("Access-Control-Allow-Origin"); got == "https://a.example.com" {
		t.Error("Allow-Origin should not be echoed when no origins are configured")
	}
}

func TestNewHelmet(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewHelmet())
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	res := do(t, app, request(t, "/"))

	if got := res.Header.Get("X-XSS-Protection"); got == "" {
		t.Error("X-XSS-Protection header is missing")
	}
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

			if res.Status != http.StatusOK {
				t.Errorf("status = %d, want 200", res.Status)
			}
			if tt.level == 0 {
				if got := res.Header.Get("Content-Encoding"); got != "" {
					t.Errorf("Content-Encoding = %q, want no compression", got)
				}
				return
			}
			if got := res.Header.Get("Content-Encoding"); got != "gzip" {
				t.Errorf("Content-Encoding = %q, want gzip", got)
			}
			if res.Body == "" {
				t.Error("body is empty")
			}
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

	if res.Header.Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header is missing")
	}
	if res.Body == "" {
		t.Error("the request id was not stored in the context")
	}
	if res.Body != res.Header.Get("X-Request-ID") {
		t.Errorf("context id = %q, want it to match the header %q", res.Body, res.Header.Get("X-Request-ID"))
	}
}

func TestRequestLogger(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewRequestID())
	app.Use(middleware.RequestLogger(slog.Default()))
	app.Get("/ok", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	res := do(t, app, request(t, "/ok"))

	if res.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", res.Status)
	}
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

	if res.Status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", res.Status)
	}
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

	if res.Status != http.StatusOK {
		t.Errorf("status = %d, want 200", res.Status)
	}
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

	if res.Body != "Europe/Berlin" {
		t.Errorf("timezone = %q, want %q", res.Body, "Europe/Berlin")
	}
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

	if res.Body != tzJakarta {
		t.Errorf("timezone = %q, want %q", res.Body, tzJakarta)
	}
}

func TestNewTimezone_InvalidDefaultFallsBackToUTC(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.NewTimezone("Not/AZone"))
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(ctxvalTimezone(c))
	})

	res := do(t, app, request(t, "/"))

	if res.Body != "UTC" {
		t.Errorf("timezone = %q, want %q", res.Body, "UTC")
	}
}

func TestNewErrorHandler(t *testing.T) {
	app := fiber.New(fiber.Config{
		ErrorHandler: middleware.NewErrorHandler(slog.Default()),
	})
	app.Get("/boom", func(_ fiber.Ctx) error {
		return fiber.NewError(http.StatusTeapot, "boom")
	})

	res := do(t, app, request(t, "/boom"))

	if res.Status != http.StatusTeapot {
		t.Errorf("status = %d, want 418", res.Status)
	}
	if !strings.Contains(res.Body, "boom") {
		t.Errorf("body = %q, want it to contain the error detail", res.Body)
	}
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

	if res.Status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", res.Status)
	}
	if strings.Contains(res.Body, "hunter2") {
		t.Errorf("body = %q, want it to not leak the internal error", res.Body)
	}
}

func TestBadRequestResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return response.BadRequestResponse(c, "Invalid payload", "the body is malformed", []response.InvalidParam{
			{Name: "name", Reason: "is required"},
		})
	})

	res := do(t, app, request(t, "/"))

	if res.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.Status)
	}
	if !strings.Contains(res.Body, "is required") {
		t.Errorf("body = %q, want it to contain the invalid parameter", res.Body)
	}
}

func TestUnprocessableEntityResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return response.UnprocessableEntityResponse(c, "Unprocessable", "the balance does not add up")
	})

	res := do(t, app, request(t, "/"))

	if res.Status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", res.Status)
	}
	if !strings.Contains(res.Body, "does not add up") {
		t.Errorf("body = %q, want it to contain the detail", res.Body)
	}
}
