package jwt

import "time"

// Payload contains the core claims stored in the JWT.
type Payload struct {
	UserID       string `json:"user_id"`
	TokenVersion int    `json:"token_version"`
}

// Response represents the result of token generation.
type Response struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Service defines operations for JWT token management.
type Service interface {
	// GenerateToken creates a new JWT token with the given payload.
	// The token expires according to the configured duration.
	GenerateToken(payload Payload) (*Response, error)

	// ValidateToken verifies and decodes a JWT token string.
	// Returns the payload if valid, or an error if invalid or expired.
	ValidateToken(token string) (*Payload, error)
}
