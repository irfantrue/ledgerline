package responsetest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ledgerline/internal/shared/httpx/response"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func get(t *testing.T, h fiber.Handler) (*http.Response, string) {
	t.Helper()

	app := fiber.New()
	app.Get("/x", h)

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil))
	require.NoError(t, err, "app.Test")
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body")

	return resp, string(b)
}

// Regression: `omitempty` on Data silently dropped 0 / false / "".
func TestWriteSuccess_KeepsZeroValues(t *testing.T) {
	resp, body := get(t, func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[int]{Status: http.StatusOK, Data: 0})
	})
	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, body, `"data":0`, "the zero value should not be dropped from the body")
}

func TestWriteSuccess_StatusAndContentType(t *testing.T) {
	resp, _ := get(t, func(c fiber.Ctx) error {
		return response.WriteSuccess(c, &response.Response[string]{Status: http.StatusCreated, Data: "ok"})
	})
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "status")
	assert.Contains(t, resp.Header.Get("Content-Type"), response.ContentType, "content-type")
}

func TestWriteProblem_UsesProblemJSON(t *testing.T) {
	resp, body := get(t, func(c fiber.Ctx) error {
		return response.UnauthorizedResponse(c, "token expired")
	})
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "status")
	assert.Contains(t, resp.Header.Get("Content-Type"), response.ContentType, "content-type")
	assert.Contains(t, body, "token expired", "the problem detail should be in the body")
}

func TestIsProblemAndFrom(t *testing.T) {
	p := &response.Problem{Title: "t", Status: http.StatusBadRequest}

	assert.True(t, response.IsProblem(p), "IsProblem should be true for *response.Problem")

	got, ok := response.From(p)
	require.True(t, ok, "From should find the problem")
	assert.Same(t, p, got, "From should return the same *response.Problem")

	assert.False(t, response.IsProblem(io.EOF), "IsProblem should be false for other errors")
}
