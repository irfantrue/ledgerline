package middlewaretest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ledgerline/internal/shared/httpx/middleware"
	"ledgerline/pkg/ctxval"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipWithoutTZData(t *testing.T) {
	t.Helper()
	if _, err := time.LoadLocation(tzJakarta); err != nil {
		t.Skipf("tzdata not available: %v", err)
	}
}

func TestLoadLocation_CachesAndRejectsInvalid(t *testing.T) {
	skipWithoutTZData(t)

	first, ok := middleware.LoadLocation(tzJakarta)
	require.True(t, ok, "%s should load", tzJakarta)

	second, ok := middleware.LoadLocation(tzJakarta)
	require.True(t, ok, "%s should load on the second lookup", tzJakarta)
	assert.Same(t, first, second, "the second lookup should return the cached *time.Location")

	_, ok = middleware.LoadLocation("Not/AZone")
	assert.False(t, ok, "an invalid zone must not load")
}

func TestTimezoneMiddleware(t *testing.T) {
	skipWithoutTZData(t)

	app := fiber.New()
	app.Use(middleware.NewTimezone(tzJakarta))
	app.Get("/tz", func(c fiber.Ctx) error {
		return c.SendString(ctxval.GetTimezone(c.Context()).String())
	})

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "default when header missing", header: "", want: tzJakarta},
		{name: "valid header wins", header: "America/New_York", want: "America/New_York"},
		{name: "invalid header falls back to default", header: "Nope/Nope", want: tzJakarta},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tz", nil)
			if tt.header != "" {
				req.Header.Set("X-Timezone", tt.header)
			}
			res := do(t, app, req)
			assert.Equal(t, tt.want, res.Body, "timezone")
		})
	}
}
