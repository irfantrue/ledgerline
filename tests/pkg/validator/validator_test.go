package validatortest

import (
	"testing"

	"ledgerline/pkg/validator"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payload struct {
	Name  string `json:"name" validate:"required,min=3"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0,lte=150"`
}

func TestValidateStruct_Valid(t *testing.T) {
	got := validator.ValidateStruct(payload{
		Name:  "alice",
		Email: "alice@example.com",
		Age:   30,
	})
	assert.Empty(t, got, "ValidateStruct() = %v, want no errors", got)
}

func TestValidateStruct_ReportsEveryViolation(t *testing.T) {
	got := validator.ValidateStruct(payload{})

	// age 0 satisfies gte=0, so only name and email are reported.
	require.Len(t, got, 2, "len(errors) = %d, want 2: %+v", len(got), got)

	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
		assert.NotEmpty(t, p.Reason, "error for %q has an empty reason", p.Name)
	}

	for _, want := range []string{"name", "email"} {
		assert.True(t, names[want], "no error reported for field %q", want)
	}
}

func TestValidateStruct_NumericRange(t *testing.T) {
	got := validator.ValidateStruct(payload{
		Name:  "alice",
		Email: "alice@example.com",
		Age:   200,
	})

	require.Len(t, got, 1, "len(errors) = %d, want 1: %+v", len(got), got)
	assert.Equal(t, "age", got[0].Name, "Name = %q, want %q", got[0].Name, "age")
}

// TestValidateStruct_UsesJSONFieldNames checks that the reported field names
// come from the json tag, not from the Go field name.
func TestValidateStruct_UsesJSONFieldNames(t *testing.T) {
	type renamed struct {
		DisplayName string `json:"display_name" validate:"required"`
	}

	got := validator.ValidateStruct(renamed{})
	require.Len(t, got, 1, "len(errors) = %d, want 1: %+v", len(got), got)
	assert.Equal(t, "display_name", got[0].Name, "Name = %q, want %q", got[0].Name, "display_name")
}

// TestValidateStruct_NonStructInput covers the branch where the payload is
// not a struct at all: validator returns an InvalidValidationError, which the
// package reports as a single "payload" error instead of panicking.
func TestValidateStruct_NonStructInput(t *testing.T) {
	got := validator.ValidateStruct("not a struct")

	require.Len(t, got, 1, "len(errors) = %d, want 1: %+v", len(got), got)
	assert.Equal(t, "payload", got[0].Name, "Name = %q, want %q", got[0].Name, "payload")
	assert.NotEmpty(t, got[0].Reason, "Reason is empty")
}
