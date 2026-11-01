package middlewaretest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ledgerline/internal/shared/httpx/middleware"

	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
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
	if strings.Contains(string(body), "secret_user") || strings.Contains(string(body), "pq:") {
		t.Errorf("internal error leaked to client: %s", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
}

func TestErrorHandler_ProblemPassthrough(t *testing.T) {
	prob := &response.Problem{Title: "Invalid", Status: http.StatusUnprocessableEntity, Detail: "amount must be greater than 0"}
	app := newErrApp(func(fiber.Ctx) error { return fmt.Errorf("usecase: %w", prob) })

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

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	if !strings.Contains(string(body), "amount must be greater than 0") {
		t.Errorf("problem detail missing from body: %s", body)
	}
}

func TestErrorHandler_FiberErrors(t *testing.T) {
	app := newErrApp(func(fiber.Ctx) error {
		return fmt.Errorf("wrapped: %w", fiber.NewError(http.StatusConflict, "already exists"))
	})

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

	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "already exists") {
		t.Errorf("wrapped fiber.Error not honoured: %d %s", resp.StatusCode, body)
	}

	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/does-not-exist", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("unknown route status = %d, want 404", resp2.StatusCode)
	}
}
