package middlewaretest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/httpx/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testOrigin = "http://a.com"

func TestParseOrigins(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty input", in: "", want: nil},
		{name: "single origin", in: testOrigin, want: []string{testOrigin}},
		{name: "comma separated", in: "http://a.com,http://b.com", want: []string{"http://a.com", "http://b.com"}},
		{name: "padded and trailing comma", in: " http://a.com ,  http://b.com ,", want: []string{"http://a.com", "http://b.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, middleware.ParseOrigins(tt.in), "ParseOrigins(%q)", tt.in)
		})
	}
}

// allowOrigin runs a request through the cors middleware and returns the
// Access-Control-Allow-Origin header it produced.
func allowOrigin(t *testing.T, cfg config.Config, origin string) string {
	t.Helper()

	app := fiber.New()
	app.Use(middleware.NewCors(cfg))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("Origin", origin)

	resp, err := app.Test(req)
	require.NoError(t, err, "request")
	defer func() { _ = resp.Body.Close() }()

	return resp.Header.Get("Access-Control-Allow-Origin")
}

// TestNewCors_ProductionWithoutOrigins checks that production does not fall
// back to "every origin" when no origin is configured. The cors middleware
// treats an empty list as "allow all", so the origins have to be closed.
func TestNewCors_ProductionWithoutOrigins(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{Env: "production"}}

	assert.Empty(t, allowOrigin(t, cfg, testOrigin), "no Access-Control-Allow-Origin header should be sent in production")
}

// TestNewCors_ProductionWithOrigins checks that the configured origins are
// still served in production.
func TestNewCors_ProductionWithOrigins(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{
		Env:                "production",
		CorsAllowedOrigins: testOrigin,
	}}

	assert.Equal(t, testOrigin, allowOrigin(t, cfg, testOrigin), "the configured origin should be allowed")
	assert.Empty(t, allowOrigin(t, cfg, "http://b.com"), "an unlisted origin should not be allowed")
}

// TestNewCors_DevelopmentWithoutOrigins checks the development default: every
// origin is allowed.
func TestNewCors_DevelopmentWithoutOrigins(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{Env: "development"}}

	assert.Equal(t, "*", allowOrigin(t, cfg, testOrigin), "every origin should be allowed in development")
}

// TestNewCors_WildcardOriginDoesNotPanic checks that a wildcard in the origin
// list does not combine with credentials. The cors middleware panics on that
// combination, so the middleware has to turn the credentials off.
func TestNewCors_WildcardOriginDoesNotPanic(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{
		Env:                "development",
		CorsAllowedOrigins: "*",
	}}

	assert.NotPanics(t, func() {
		assert.Equal(t, "*", allowOrigin(t, cfg, testOrigin), "the wildcard origin should be allowed")
	}, "NewCors() should not panic on a wildcard origin")
}
