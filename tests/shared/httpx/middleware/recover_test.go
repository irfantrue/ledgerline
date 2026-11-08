package middlewaretest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledgerline/internal/shared/httpx/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecover_ReturnsProblemAndHidesPanicValue(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: middleware.NewErrorHandler(discardLogger())})
	app.Use(middleware.NewRecover(discardLogger()))
	app.Get("/x", func(fiber.Ctx) error { panic("kaboom-internal") })

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err, "app.Test")
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body")

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode, "status")
	assert.NotContains(t, string(body), "kaboom-internal", "the panic value should not leak to the client")
}
