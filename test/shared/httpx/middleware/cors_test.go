package middlewaretest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ledgerline/internal/shared/config"
	"ledgerline/internal/shared/httpx/middleware"

	"github.com/gofiber/fiber/v3"
)

const testOrigin = "http://a.com"

func TestParseOrigins(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{testOrigin, []string{testOrigin}},
		{"http://a.com,http://b.com", []string{"http://a.com", "http://b.com"}},
		{" http://a.com ,  http://b.com ,", []string{"http://a.com", "http://b.com"}},
	}

	for _, tt := range tests {
		got := middleware.ParseOrigins(tt.in)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("middleware.ParseOrigins(%q) = %v, want %v", tt.in, got, tt.want)
		}
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
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.Header.Get("Access-Control-Allow-Origin")
}

// TestNewCors_ProductionWithoutOrigins checks that production does not fall
// back to "every origin" when no origin is configured. The cors middleware
// treats an empty list as "allow all", so the origins have to be closed.
func TestNewCors_ProductionWithoutOrigins(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{Env: "production"}}

	if got := allowOrigin(t, cfg, testOrigin); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want no header in production", got)
	}
}

// TestNewCors_ProductionWithOrigins checks that the configured origins are
// still served in production.
func TestNewCors_ProductionWithOrigins(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{
		Env:                "production",
		CorsAllowedOrigins: testOrigin,
	}}

	if got := allowOrigin(t, cfg, testOrigin); got != testOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, testOrigin)
	}
	if got := allowOrigin(t, cfg, "http://b.com"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want no header for an unlisted origin", got)
	}
}

// TestNewCors_DevelopmentWithoutOrigins checks the development default: every
// origin is allowed.
func TestNewCors_DevelopmentWithoutOrigins(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{Env: "development"}}

	if got := allowOrigin(t, cfg, testOrigin); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

// TestNewCors_WildcardOriginDoesNotPanic checks that a wildcard in the origin
// list does not combine with credentials. The cors middleware panics on that
// combination, so the middleware has to turn the credentials off.
func TestNewCors_WildcardOriginDoesNotPanic(t *testing.T) {
	cfg := config.Config{App: config.AppConfig{
		Env:                "development",
		CorsAllowedOrigins: "*",
	}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewCors() panicked: %v", r)
		}
	}()

	if got := allowOrigin(t, cfg, testOrigin); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}
