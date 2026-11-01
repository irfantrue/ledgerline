package middlewaretest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// result is what a test request produced.
type result struct {
	Status int
	Header http.Header
	Body   string
}

// request builds a GET request for the given path.
func request(t testing.TB, path string) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
}

// do runs a request through the app and returns the status, the headers and
// the body. The response is closed here, so it never escapes this helper.
func do(t testing.TB, app *fiber.App, req *http.Request) result {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return result{Status: resp.StatusCode, Header: resp.Header, Body: string(body)}
}
