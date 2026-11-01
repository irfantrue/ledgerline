package response

import (
	"errors"
	"fmt"

	"ledgerline/pkg/validator"
)

const (
	// ContentType is the media type for successful JSON responses.
	ContentType = "application/json"
)

// Response is the standard API response envelope for all successful responses.
type Response[T any] struct {
	Status int `json:"-"`
	Data   T   `json:"data"` // no omitempty: it would drop legit zero values (0, false, "")
}

// InvalidParam is a type alias for validator.InvalidParam.
// It represents a validation error detail for a specific field.
type InvalidParam = validator.InvalidParam

// Problem is the root RFC 7807 problem detail object.
// It provides a standardized format for describing API errors.
type Problem struct {
	Title         string         `json:"title"`
	Status        int            `json:"status"`
	Detail        string         `json:"detail,omitempty"`
	InvalidParams []InvalidParam `json:"invalid_params,omitempty"`
}

// Error implements the error interface for the Problem struct.
func (p *Problem) Error() string {
	return fmt.Sprintf("[%d] %s — %s", p.Status, p.Title, p.Detail)
}

// IsProblem reports whether err is or wraps a *Problem.
func IsProblem(err error) bool {
	var p *Problem
	return errors.As(err, &p)
}

// From extracts a *Problem from err. Returns nil if not found.
func From(err error) (*Problem, bool) {
	return errors.AsType[*Problem](err)
}
