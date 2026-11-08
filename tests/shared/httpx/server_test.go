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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFiber(t *testing.T) {
	app := httpx.NewFiber(config.Config{
		App: config.AppConfig{
			AppName:      appName,
			ServerHeader: appName,
		},
	}, slog.Default())

	require.NotNil(t, app, "NewFiber() should return an app")

	app.Get("/ping", func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[string]{Status: http.StatusOK, Data: "pong"})
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
	res, err := app.Test(req)
	require.NoError(t, err, "app.Test")
	defer func() { _ = res.Body.Close() }()

	assert.Equal(t, http.StatusOK, res.StatusCode, "status")
	assert.Equal(t, appName, res.Header.Get("Server"), "Server header")
}

// TestNewFiber_NilLogger makes sure a missing logger falls back to the
// default one instead of panicking.
func TestNewFiber_NilLogger(t *testing.T) {
	app := httpx.NewFiber(config.Config{}, nil)

	require.NotNil(t, app, "NewFiber() should return an app")

	app.Get("/ping", func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[string]{Status: http.StatusOK, Data: "pong"})
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil)
	res, err := app.Test(req)
	require.NoError(t, err, "app.Test")
	defer func() { _ = res.Body.Close() }()

	assert.Equal(t, http.StatusOK, res.StatusCode, "status")
}
