package middlewaretest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledgerline/internal/shared/httpx/middleware"
	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newErrApp(handler fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: middleware.NewErrorHandler(discardLogger())})
	app.Get("/x", handler)
	return app
}

// Regression: 500s used to echo err.Error() (SQL text, paths, ...) to the client.
func TestErrorHandler_DoesNotLeakInternalErrors(t *testing.T) {
	app := newErrApp(func(fiber.Ctx) error {
		return errors.New(`pq: password authentication failed for user "secret_user"`)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err, "app.Test")
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body")

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode, "status")
	assert.NotContains(t, string(body), "secret_user", "the internal error should not leak to the client")
	assert.NotContains(t, string(body), "pq:", "the internal error should not leak to the client")
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json", "content-type")
}

func TestErrorHandler_ProblemPassthrough(t *testing.T) {
	prob := &response.Problem{Title: "Invalid", Status: http.StatusUnprocessableEntity, Detail: "amount must be greater than 0"}
	app := newErrApp(func(fiber.Ctx) error { return fmt.Errorf("usecase: %w", prob) })

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err, "app.Test")
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body")

	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "status")
	assert.Contains(t, string(body), "amount must be greater than 0", "the problem detail should be in the body")
}

func TestErrorHandler_FiberErrors(t *testing.T) {
	app := newErrApp(func(fiber.Ctx) error {
		return fmt.Errorf("wrapped: %w", fiber.NewError(http.StatusConflict, "already exists"))
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err, "app.Test")
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body")

	require.Equal(t, http.StatusConflict, resp.StatusCode, "status")
	assert.Contains(t, string(body), "already exists", "the error message should be in the body")

	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/does-not-exist", nil)
	resp2, err := app.Test(req2)
	require.NoError(t, err, "app.Test")
	defer func() { _ = resp2.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp2.StatusCode, "unknown route status")
}
