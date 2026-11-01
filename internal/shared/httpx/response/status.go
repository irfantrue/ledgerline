package response

import (
	"net/http"

	"github.com/gofiber/fiber/v3"
)

// UnauthorizedResponse returns a standard RFC 7807 problem detail response
// for unauthorized access (401).
func UnauthorizedResponse(c fiber.Ctx, detail string) error {
	return WriteProblem(c, &Problem{
		Title:  "Unauthorized Access",
		Status: http.StatusUnauthorized,
		Detail: detail,
	})
}

// BadRequestResponse returns a standard RFC 7807 problem detail response
// for bad request (400).
func BadRequestResponse(c fiber.Ctx, title, detail string, invalidParams []InvalidParam) error {
	return WriteProblem(c, &Problem{
		Title:         title,
		Status:        http.StatusBadRequest,
		Detail:        detail,
		InvalidParams: invalidParams,
	})
}

// UnprocessableEntityResponse returns a standard RFC 7807 problem detail response
// for unprocessable entity (422).
func UnprocessableEntityResponse(c fiber.Ctx, title, detail string) error {
	return WriteProblem(c, &Problem{
		Title:  title,
		Status: http.StatusUnprocessableEntity,
		Detail: detail,
	})
}
