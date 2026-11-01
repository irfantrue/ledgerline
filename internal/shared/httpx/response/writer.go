package response

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
)

// writeJSON is an internal helper that writes a JSON response with the given status code.
func writeJSON(c fiber.Ctx, status int, contentType string, body any) error {
	c.Status(status)
	if err := c.JSON(body, contentType); err != nil {
		return fmt.Errorf("failed to write JSON response: %w", err)
	}
	return nil
}

// WriteProblem sends a Problem (RFC 7807) response to the client.
func WriteProblem(c fiber.Ctx, p *Problem) error {
	return writeJSON(c, p.Status, ContentType, p)
}

// WriteSuccess sends a standard API response to the client.
func WriteSuccess[T any](c fiber.Ctx, r *Response[T]) error {
	return writeJSON(c, r.Status, ContentType, r)
}
