package validator

// InvalidParam represents a validation error detail for a specific field.
type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}
