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
	"ledgerline/tests/mocks/httpxmocks"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "app.Test")
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err, "read body")

	return res.StatusCode, string(body)
}

// TestRegisterRoutes_Healthz checks the liveness probe.
func TestRegisterRoutes_Healthz(t *testing.T) {
	app := newApp(t)

	status, body := send(t, app, "/healthz")

	assert.Equal(t, http.StatusOK, status, "status")
	assert.NotEmpty(t, body, "body should not be empty")
}

// TestRegisterRoutes_Readyz checks the readiness probe without a database.
func TestRegisterRoutes_Readyz(t *testing.T) {
	app := newApp(t)

	status, body := send(t, app, "/readyz")

	assert.Equal(t, http.StatusOK, status, "status")
	assert.NotEmpty(t, body, "body should not be empty")
}

// TestRegisterRoutes_Readyz_UnavailableDatabase checks that the readiness
// probe fails when the database cannot be reached.
func TestRegisterRoutes_Readyz_UnavailableDatabase(t *testing.T) {
	db := &httpxmocks.Pinger{}
	db.On("Ping", mock.Anything).Return(errors.New("database is down")).Once()

	app := httpx.NewFiber(config.Config{}, slog.Default())

	httpx.RegisterRoutes(&httpx.Config{
		App:    app,
		Config: config.Config{},
		Logger: slog.Default(),
		DB:     db,
	})

	status, _ := send(t, app, "/readyz")

	assert.Equal(t, http.StatusServiceUnavailable, status, "status")
	db.AssertExpectations(t)
}

// TestRegisterRoutes_Recover checks that a panic is turned into a Problem
// response instead of taking the process down.
func TestRegisterRoutes_Recover(t *testing.T) {
	app := newApp(t)

	app.Get("/panic", func(fiber.Ctx) error {
		panic("boom")
	})

	status, body := send(t, app, "/panic")

	assert.Equal(t, http.StatusInternalServerError, status, "status")
	assert.NotEmpty(t, body, "body should not be empty")
}

// TestRegisterRoutes_NotFound checks the default Fiber 404 handling.
func TestRegisterRoutes_NotFound(t *testing.T) {
	app := newApp(t)

	status, _ := send(t, app, "/does-not-exist")

	assert.Equal(t, http.StatusNotFound, status, "status")
}
