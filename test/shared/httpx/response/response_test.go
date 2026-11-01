package responsetest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
)

func get(t *testing.T, h fiber.Handler) (*http.Response, string) {
	t.Helper()

	app := fiber.New()
	app.Get("/x", h)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(b)
}

// Regression: `omitempty` on Data silently dropped 0 / false / "".
func TestWriteSuccess_KeepsZeroValues(t *testing.T) {
	resp, body := get(t, func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[int]{Status: http.StatusOK, Data: 0})
	})
	defer func() { _ = resp.Body.Close() }()

	if !strings.Contains(body, `"data":0`) {
		t.Errorf("zero value dropped from body: %s", body)
	}
}

func TestWriteSuccess_StatusAndContentType(t *testing.T) {
	resp, body := get(t, func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[string]{Status: http.StatusCreated, Data: "ok"})
	})
	defer func() { _ = resp.Body.Close() }()
	_ = body // silence unused variable

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, response.ContentType) {
		t.Errorf("content-type = %q", ct)
	}
}

func TestWriteProblem_UsesProblemJSON(t *testing.T) {
	resp, body := get(t, func(c fiber.Ctx) error {
		return response.UnauthorizedResponse(c, "token expired")
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, response.ContentType) {
		t.Errorf("content-type = %q, want %s", ct, response.ContentType)
	}
	if !strings.Contains(body, "token expired") {
		t.Errorf("detail missing: %s", body)
	}
}

func TestIsProblemAndFrom(t *testing.T) {
	p := &response.Problem{Title: "t", Status: http.StatusBadRequest}

	if !response.IsProblem(p) {
		t.Error("IsProblem should be true for *response.Problem")
	}
	if got, ok := response.From(p); !ok || got != p {
		t.Error("From should return the same *response.Problem")
	}
	if response.IsProblem(io.EOF) {
		t.Error("IsProblem should be false for other errors")
	}
}
