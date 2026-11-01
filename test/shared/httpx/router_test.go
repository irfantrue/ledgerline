package httptest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/httpx"

	"github.com/gofiber/fiber/v3"
)

// appName is the application name the tests configure.
const appName = "ledgerline-api"

// newApp builds a Fiber app with the production routes registered.
func newApp(t *testing.T) *fiber.App {
	t.Helper()

	app := httpx.NewFiber(config.Config{
		App: config.AppConfig{AppName: appName, ServerHeader: appName},
	}, slog.Default())

	httpx.RegisterRoutes(&httpx.Config{
		App:    app,
		Config: config.Config{},
		Logger: slog.Default(),
	})

	return app
}

// send performs a GET request and returns the status code and the body.
func send(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	return res.StatusCode, string(body)
}

// TestRegisterRoutes_Healthz checks the liveness probe.
func TestRegisterRoutes_Healthz(t *testing.T) {
	app := newApp(t)

	status, body := send(t, app, "/healthz")

	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if body == "" {
		t.Error("body is empty")
	}
}

// TestRegisterRoutes_Readyz checks the readiness probe without a database.
func TestRegisterRoutes_Readyz(t *testing.T) {
	app := newApp(t)

	status, body := send(t, app, "/readyz")

	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if body == "" {
		t.Error("body is empty")
	}
}

// TestRegisterRoutes_Readyz_UnavailableDatabase checks that the readiness
// probe fails when the database cannot be reached.
func TestRegisterRoutes_Readyz_UnavailableDatabase(t *testing.T) {
	app := httpx.NewFiber(config.Config{}, slog.Default())

	httpx.RegisterRoutes(&httpx.Config{
		App:    app,
		Config: config.Config{},
		Logger: slog.Default(),
		DB:     failingPinger{},
	})

	status, _ := send(t, app, "/readyz")

	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", status)
	}
}

// TestRegisterRoutes_Recover checks that a panic is turned into a Problem
// response instead of taking the process down.
func TestRegisterRoutes_Recover(t *testing.T) {
	app := newApp(t)

	app.Get("/panic", func(fiber.Ctx) error {
		panic("boom")
	})

	status, body := send(t, app, "/panic")

	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
	if body == "" {
		t.Error("body is empty")
	}
}

// TestRegisterRoutes_NotFound checks the default Fiber 404 handling.
func TestRegisterRoutes_NotFound(t *testing.T) {
	app := newApp(t)

	status, _ := send(t, app, "/does-not-exist")

	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

// failingPinger always fails to ping.
type failingPinger struct{}

func (failingPinger) Ping(_ context.Context) error {
	return errors.New("database is down")
}
