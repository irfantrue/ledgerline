package idgen

import "github.com/google/uuid"

// New generates a new UUID v7 string.
// UUID v7 provides time-ordered unique identifiers for efficient indexing.
func New() string {
	return uuid.Must(uuid.NewV7()).String()
}
