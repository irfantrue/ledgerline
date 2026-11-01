package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// NewRequestID returns a Fiber middleware that adds a unique request ID to each request.
// The request ID is stored in the context and can be retrieved via requestid.FromContext(c.Context()).
// It also sets the X-Request-ID header in the response.
func NewRequestID() fiber.Handler {
	return requestid.New()
}
