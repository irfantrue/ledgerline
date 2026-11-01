package middlewaretest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ledgerline/internal/shared/httpx/middleware"

	"github.com/gofiber/fiber/v3"
)

func TestRecover_ReturnsProblemAndHidesPanicValue(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: middleware.NewErrorHandler(discardLogger())})
	app.Use(middleware.NewRecover(discardLogger()))
	app.Get("/x", func(fiber.Ctx) error { panic("kaboom-internal") })

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if strings.Contains(string(body), "kaboom-internal") {
		t.Errorf("panic value leaked to client: %s", body)
	}
}
