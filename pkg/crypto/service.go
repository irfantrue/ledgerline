package crypto

// CryptographyService provides methods for cryptographic operations.
type CryptographyService interface {
	// HashPassword hashes a password using Argon2id and returns the encoded PHC string.
	HashPassword(password string) (string, error)

	// ComparePassword verifies a password against an Argon2id-encoded hash.
	ComparePassword(hashedPassword, password string) error
}
