package validatortest

import (
	"testing"

	"ledgerline/pkg/validator"
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
	if len(got) != 0 {
		t.Errorf("ValidateStruct() = %v, want no errors", got)
	}
}

func TestValidateStruct_ReportsEveryViolation(t *testing.T) {
	got := validator.ValidateStruct(payload{})

	// age 0 satisfies gte=0, so only name and email are reported.
	if len(got) != 2 {
		t.Fatalf("len(errors) = %d, want 2: %+v", len(got), got)
	}

	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
		if p.Reason == "" {
			t.Errorf("error for %q has an empty reason", p.Name)
		}
	}

	for _, want := range []string{"name", "email"} {
		if !names[want] {
			t.Errorf("no error reported for field %q", want)
		}
	}
}

func TestValidateStruct_NumericRange(t *testing.T) {
	got := validator.ValidateStruct(payload{
		Name:  "alice",
		Email: "alice@example.com",
		Age:   200,
	})

	if len(got) != 1 {
		t.Fatalf("len(errors) = %d, want 1: %+v", len(got), got)
	}
	if got[0].Name != "age" {
		t.Errorf("Name = %q, want %q", got[0].Name, "age")
	}
}

// TestValidateStruct_UsesJSONFieldNames checks that the reported field names
// come from the json tag, not from the Go field name.
func TestValidateStruct_UsesJSONFieldNames(t *testing.T) {
	type renamed struct {
		DisplayName string `json:"display_name" validate:"required"`
	}

	got := validator.ValidateStruct(renamed{})
	if len(got) != 1 {
		t.Fatalf("len(errors) = %d, want 1: %+v", len(got), got)
	}
	if got[0].Name != "display_name" {
		t.Errorf("Name = %q, want %q", got[0].Name, "display_name")
	}
}

// TestValidateStruct_NonStructInput covers the branch where the payload is
// not a struct at all: validator returns an InvalidValidationError, which the
// package reports as a single "payload" error instead of panicking.
func TestValidateStruct_NonStructInput(t *testing.T) {
	got := validator.ValidateStruct("not a struct")

	if len(got) != 1 {
		t.Fatalf("len(errors) = %d, want 1: %+v", len(got), got)
	}
	if got[0].Name != "payload" {
		t.Errorf("Name = %q, want %q", got[0].Name, "payload")
	}
	if got[0].Reason == "" {
		t.Error("Reason is empty")
	}
}
