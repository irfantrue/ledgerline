package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
	"golang.org/x/crypto/argon2"
)

// Sentinel errors for password hashing operations.
var (
	ErrInvalidHashFormat    = errors.New("hashed password does not match expected PHC format")
	ErrUnsupportedAlgorithm = errors.New("hash algorithm is not argon2id")
	ErrVersionMismatch      = errors.New("argon2id version in hash does not match library version")
	ErrInvalidParams        = errors.New("argon2id parameters in hash are invalid or corrupted")
	ErrParamsOutOfRange     = errors.New("argon2id parameters in hash exceed the allowed range")
	ErrCorruptedSalt        = errors.New("salt component of hash is not valid base64")
	ErrCorruptedHash        = errors.New("hash component of hash is not valid base64 or has invalid length")
	ErrIncorrectPassword    = errors.New("password does not match the stored hash")
)

// Bounds for the parameters that are read back from a stored hash. The hash
// comes from the database, so a corrupted or tampered row must not be able to
// make the process allocate gigabytes of memory or spawn hundreds of threads.
//
// The defaults of NewCryptographyService are 64 MiB of memory, 3 iterations,
// 2 threads, a 16 byte salt and a 32 byte key, so these limits leave room for a
// stronger configuration while refusing the absurd ones.
const (
	maxHashMemory      = 256 * 1024 // KiB, i.e. 256 MiB
	maxHashIterations  = 10
	maxHashParallelism = 16
	maxHashSaltLength  = 64
	maxHashKeyLength   = 64
	// maxHashLength is the longest PHC string that is parsed at all.
	maxHashLength = 1024
)

// params holds the configuration for Argon2id password hashing.
type params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

// hashParams holds the Argon2id parameters of a stored hash.
type hashParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

// Cryptography provides concrete implementations for cryptographic operations.
type Cryptography struct {
	p *params
}

// NewCryptographyService creates and returns a new instance of Cryptography
// with default Argon2id parameters: 64MB memory, 3 iterations, 2 parallelism,
// 16 bytes salt, and 32 bytes key length.
func NewCryptographyService() *Cryptography {
	return &Cryptography{
		p: &params{
			memory:      64 * 1024, // 64 MB memory
			iterations:  3,         // 3 iteration CPU
			parallelism: 2,         // 2 threads/cores
			saltLength:  16,        // 16 bytes length salt
			keyLength:   32,        // 32 bytes length hash
		},
	}
}

// HashPassword generates a secure hash of the given password using Argon2id.
// It returns the hashed password in PHC format "$argon2id$v=$m=,t=,p=$<$salt$<$hash"
// or an error if the hashing fails.
func (s *Cryptography) HashPassword(password string) (string, error) {
	salt := make([]byte, s.p.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate cryptographically secure salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, s.p.iterations, s.p.memory, s.p.parallelism, s.p.keyLength)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, s.p.memory, s.p.iterations, s.p.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// ComparePassword verifies if the provided password matches the Argon2id-encoded hash.
// It returns nil if the password matches, or one of the sentinel errors if verification fails.
//
// The parameters are read back from the stored hash, so they are bounded before
// argon2 is called: an unbounded value could ask for gigabytes of memory or
// hundreds of threads.
func (s *Cryptography) ComparePassword(hashedPassword, password string) error {
	p, salt, hash, err := parseHash(hashedPassword)
	if err != nil {
		return err
	}

	keyLength, err := safecast.Convert[uint32](len(hash))
	if err != nil {
		return ErrCorruptedHash
	}

	comparisonHash := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, keyLength)

	if subtle.ConstantTimeCompare(hash, comparisonHash) == 1 {
		return nil
	}
	return ErrIncorrectPassword
}

// parseHash decodes a PHC string into its parameters, its salt and its hash.
func parseHash(hashedPassword string) (hashParams, []byte, []byte, error) {
	if len(hashedPassword) > maxHashLength {
		return hashParams{}, nil, nil, ErrInvalidHashFormat
	}

	vals := strings.Split(hashedPassword, "$")
	if len(vals) != 6 {
		return hashParams{}, nil, nil, ErrInvalidHashFormat
	}
	if vals[1] != "argon2id" {
		return hashParams{}, nil, nil, ErrUnsupportedAlgorithm
	}

	// The version is parsed strictly: a trailing garbage ("v=19xyz") must not
	// pass as a valid version.
	version, err := strconv.ParseUint(strings.TrimPrefix(vals[2], "v="), 10, 64)
	if err != nil || version != argon2.Version {
		return hashParams{}, nil, nil, ErrVersionMismatch
	}

	p, err := parseParams(vals[3])
	if err != nil {
		return hashParams{}, nil, nil, err
	}

	salt, hash, err := decodeComponents(vals[4], vals[5])
	if err != nil {
		return hashParams{}, nil, nil, err
	}

	return p, salt, hash, nil
}

// parseParams reads the Argon2id parameters of a PHC string and bounds them.
func parseParams(raw string) (hashParams, error) {
	var m, t, p int
	if _, err := fmt.Sscanf(raw, "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return hashParams{}, ErrInvalidParams
	}

	memory, err := safecast.Convert[uint32](m)
	if err != nil {
		return hashParams{}, ErrInvalidParams
	}
	iterations, err := safecast.Convert[uint32](t)
	if err != nil {
		return hashParams{}, ErrInvalidParams
	}
	parallelism, err := safecast.Convert[uint8](p)
	if err != nil {
		return hashParams{}, ErrInvalidParams
	}

	if memory > maxHashMemory || iterations > maxHashIterations || parallelism > maxHashParallelism {
		return hashParams{}, ErrParamsOutOfRange
	}

	// The lower bounds matter as much as the upper ones: argon2 panics on a
	// zero iteration count or a zero parallelism, and a panic in the middle of
	// a login takes the request down instead of failing it.
	if memory == 0 || iterations == 0 || parallelism == 0 {
		return hashParams{}, ErrInvalidParams
	}

	return hashParams{memory: memory, iterations: iterations, parallelism: parallelism}, nil
}

// decodeComponents decodes the salt and the hash of a PHC string and bounds
// their length.
func decodeComponents(rawSalt, rawHash string) ([]byte, []byte, error) {
	salt, err := base64.RawStdEncoding.DecodeString(rawSalt)
	if err != nil {
		return nil, nil, ErrCorruptedSalt
	}
	if len(salt) == 0 {
		return nil, nil, ErrCorruptedSalt
	}
	if len(salt) > maxHashSaltLength {
		return nil, nil, ErrParamsOutOfRange
	}

	hash, err := base64.RawStdEncoding.DecodeString(rawHash)
	if err != nil {
		return nil, nil, ErrCorruptedHash
	}
	// An empty hash makes argon2 dereference a nil digest, so it is rejected
	// before argon2 is called.
	if len(hash) == 0 {
		return nil, nil, ErrCorruptedHash
	}
	if len(hash) > maxHashKeyLength {
		return nil, nil, ErrParamsOutOfRange
	}

	return salt, hash, nil
}
