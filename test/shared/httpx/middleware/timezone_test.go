package middlewaretest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledgerline/internal/shared/httpx/middleware"
	"time"

	"ledgerline/pkg/ctxval"

	"github.com/gofiber/fiber/v3"
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
	if !ok {
		t.Fatal("Asia/Jakarta should load")
	}
	second, _ := middleware.LoadLocation(tzJakarta)
	if first != second {
		t.Error("second lookup should return the cached *time.Location")
	}

	if _, ok := middleware.LoadLocation("Not/AZone"); ok {
		t.Error("invalid zone must not load")
	}
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
		{"default when header missing", "", tzJakarta},
		{"valid header wins", "America/New_York", "America/New_York"},
		{"invalid header falls back to default", "Nope/Nope", tzJakarta},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/tz", nil)
			if tt.header != "" {
				req.Header.Set("X-Timezone", tt.header)
			}
			res := do(t, app, req)
			if res.Body != tt.want {
				t.Errorf("timezone = %q, want %q", res.Body, tt.want)
			}
		})
	}
}
