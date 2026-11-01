package validator

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name, _, _ := strings.Cut(fld.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
}

// ValidateStruct validates a struct based on the validation tags defined on its fields.
// It returns a slice of InvalidParam if validation fails, or nil if the struct is valid.
// If the error is not a validation error (e.g., struct type mismatch), it returns a
// single InvalidParam with Name="payload" indicating an invalid request payload structure.
func ValidateStruct(data any) []InvalidParam {
	err := validate.Struct(data)
	if err == nil {
		return nil
	}

	// Safely assert the error type to avoid panics on InvalidValidationError.
	validationErrs, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return []InvalidParam{
			{
				Name:   "payload",
				Reason: "Invalid request payload structure.",
			},
		}
	}

	invalidParams := make([]InvalidParam, 0, len(validationErrs))
	for _, fieldErr := range validationErrs {
		invalidParams = append(invalidParams, InvalidParam{
			Name:   fieldErr.Field(),
			Reason: fieldErr.Error(),
		})
	}

	return invalidParams
}
