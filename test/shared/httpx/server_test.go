package httptest

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/httpx"
	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
)

func TestNewFiber(t *testing.T) {
	app := httpx.NewFiber(config.Config{
		App: config.AppConfig{
			AppName:      appName,
			ServerHeader: appName,
		},
	}, slog.Default())

	if app == nil {
		t.Fatal("NewFiber() = nil")
	}

	app.Get("/ping", func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[string]{Status: http.StatusOK, Data: "pong"})
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Server"); got != appName {
		t.Errorf("Server header = %q, want %q", got, appName)
	}
}

// TestNewFiber_NilLogger makes sure a missing logger falls back to the
// default one instead of panicking.
func TestNewFiber_NilLogger(t *testing.T) {
	app := httpx.NewFiber(config.Config{}, nil)

	if app == nil {
		t.Fatal("NewFiber() = nil")
	}

	app.Get("/ping", func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[string]{Status: http.StatusOK, Data: "pong"})
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}
