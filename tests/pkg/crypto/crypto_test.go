package cryptotest

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"ledgerline/pkg/crypto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCryptographyService(t *testing.T) {
	svc := crypto.NewCryptographyService()
	require.NotNil(t, svc, "crypto.NewCryptographyService() returned nil")
	// The service must be usable: hashing exercises the Argon2id parameters it
	// was constructed with.
	_, err := svc.HashPassword("securepassword123")
	require.NoError(t, err, "HashPassword() error = %v", err)
}

func TestHashPassword(t *testing.T) {
	svc := crypto.NewCryptographyService()

	hash, err := svc.HashPassword("securepassword123")
	require.NoError(t, err, "HashPassword() error = %v", err)
	require.NotEmpty(t, hash, "HashPassword() returned empty string")

	// Verify PHC format
	assert.True(t, strings.HasPrefix(hash, "$argon2id$v="),
		"HashPassword() = %q, want PHC format starting with $argon2id$v=", hash)
}

func TestComparePassword_Success(t *testing.T) {
	svc := crypto.NewCryptographyService()

	password := "securepassword123"
	hash, err := svc.HashPassword(password)
	require.NoError(t, err, "HashPassword() error = %v", err)

	err = svc.ComparePassword(hash, password)
	require.NoError(t, err, "ComparePassword() error = %v, want nil", err)
}

func TestComparePassword_ErrInvalidHashFormat(t *testing.T) {
	svc := crypto.NewCryptographyService()

	tests := []struct {
		name string
		hash string
	}{
		{name: "empty", hash: ""},
		{name: "only one part", hash: "$argon2id"},
		{name: "missing parts", hash: "$argon2id$v=19$m=65536,t=3,p=2"},
		{name: "too many parts", hash: "$argon2id$v=19$m=65536,t=3,p=2$base64$base64$extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ComparePassword(tt.hash, "password")
			require.ErrorIs(t, err, crypto.ErrInvalidHashFormat,
				"ComparePassword() error = %v, want crypto.ErrInvalidHashFormat", err)
		})
	}
}

func TestComparePassword_ErrUnsupportedAlgorithm(t *testing.T) {
	svc := crypto.NewCryptographyService()

	// Valid PHC format but different algorithm
	hash := "$argon2$v=19$m=65536,t=3,p=2$YWFhYWFhYWFhYWFhYWFhYQ$YmJiYmJiYmJiYmJiYmJiQ"

	err := svc.ComparePassword(hash, "password")
	require.ErrorIs(t, err, crypto.ErrUnsupportedAlgorithm,
		"ComparePassword() error = %v, want crypto.ErrUnsupportedAlgorithm", err)
}

func TestComparePassword_ErrVersionMismatch(t *testing.T) {
	svc := crypto.NewCryptographyService()

	// Valid format but wrong version
	hash := "$argon2id$v=99$m=65536,t=3,p=2$YWFhYWFhYWFhYWFhYWFhYQ$YmJiYmJiYmJiYmJiYmJiQ"

	err := svc.ComparePassword(hash, "password")
	require.ErrorIs(t, err, crypto.ErrVersionMismatch,
		"ComparePassword() error = %v, want crypto.ErrVersionMismatch", err)
}

func TestComparePassword_ErrInvalidParams(t *testing.T) {
	svc := crypto.NewCryptographyService()

	tests := []struct {
		name string
		hash string
	}{
		{
			name: "malformed params string",
			hash: "$argon2id$v=19$invalid-params$YWFhYWFhYWFhYWFhYWFhYQ$YmJiYmJiYmJiYmJiYmJiQ",
		},
		{
			name: "memory overflow - safecast fails",
			hash: "$argon2id$v=19$m=9999999999,t=3,p=2$YWFhYWFhYWFhYWFhYWFhYQ$YmJiYmJiYmJiYmJiYmJiQ",
		},
		{
			name: "parallelism overflow - safecast fails",
			hash: "$argon2id$v=19$m=65536,t=3,p=999$YWFhYWFhYWFhYWFhYWFhYQ$YmJiYmJiYmJiYmJiYmJiQ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ComparePassword(tt.hash, "password")
			require.ErrorIs(t, err, crypto.ErrInvalidParams,
				"ComparePassword() error = %v, want crypto.ErrInvalidParams", err)
		})
	}
}

// TestComparePassword_ErrParamsOutOfRange checks that the parameters read
// back from a stored hash are bounded before argon2 is called. The hash is
// real input: it comes from the database, so a corrupted or tampered row must
// not be able to make the process allocate an unreasonable amount of memory
// or spawn an unreasonable number of threads.
func TestComparePassword_ErrParamsOutOfRange(t *testing.T) {
	svc := crypto.NewCryptographyService()

	// The salt and the hash are valid base64, so the format checks pass and
	// the parameter bounds are what rejects these.
	salt := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte("A"), 16))
	hash := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte("B"), 32))
	longSalt := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte("A"), 65))
	longHash := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte("B"), 66))

	tests := []struct {
		name string
		hash string
	}{
		{
			name: "memory just above the limit",
			hash: "$argon2id$v=19$m=262145,t=3,p=2$" + salt + "$" + hash,
		},
		{
			name: "iterations just above the limit",
			hash: "$argon2id$v=19$m=65536,t=11,p=2$" + salt + "$" + hash,
		},
		{
			name: "parallelism just above the limit",
			hash: "$argon2id$v=19$m=65536,t=3,p=17$" + salt + "$" + hash,
		},
		{
			name: "salt longer than the limit",
			hash: "$argon2id$v=19$m=65536,t=3,p=2$" + longSalt + "$" + hash,
		},
		{
			name: "hash longer than the key length limit",
			hash: "$argon2id$v=19$m=65536,t=3,p=2$" + salt + "$" + longHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ComparePassword(tt.hash, "password")
			require.ErrorIs(t, err, crypto.ErrParamsOutOfRange,
				"ComparePassword() error = %v, want crypto.ErrParamsOutOfRange", err)
		})
	}
}

// TestComparePassword_RejectsDegenerateParameters checks the lower bounds of
// the parameters read back from a stored hash. The hash is real input: it
// comes from the database, so a corrupted or tampered row must not be able to
// make argon2 panic. argon2 dereferences a nil digest when the key length is
// zero, and it panics outright when the iteration count or the parallelism is
// zero, so every one of these has to be rejected before argon2 is called.
func TestComparePassword_RejectsDegenerateParameters(t *testing.T) {
	svc := crypto.NewCryptographyService()

	salt := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte("A"), 16))
	hash := base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte("B"), 32))

	tests := []struct {
		name string
		hash string
		want error
	}{
		{
			name: "empty salt",
			hash: "$argon2id$v=19$m=65536,t=3,p=2$$" + hash,
			want: crypto.ErrCorruptedSalt,
		},
		{
			name: "empty hash",
			hash: "$argon2id$v=19$m=65536,t=3,p=2$" + salt + "$",
			want: crypto.ErrCorruptedHash,
		},
		{
			name: "zero memory",
			hash: "$argon2id$v=19$m=0,t=3,p=2$" + salt + "$" + hash,
			want: crypto.ErrInvalidParams,
		},
		{
			name: "zero iterations",
			hash: "$argon2id$v=19$m=65536,t=0,p=2$" + salt + "$" + hash,
			want: crypto.ErrInvalidParams,
		},
		{
			name: "zero parallelism",
			hash: "$argon2id$v=19$m=65536,t=3,p=0$" + salt + "$" + hash,
			want: crypto.ErrInvalidParams,
		},
		{
			name: "version with trailing garbage",
			hash: "$argon2id$v=19xyz$m=65536,t=3,p=2$" + salt + "$" + hash,
			want: crypto.ErrVersionMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ComparePassword(tt.hash, "password")
			require.ErrorIs(t, err, tt.want,
				"ComparePassword() error = %v, want %v", err, tt.want)
		})
	}
}

// TestComparePassword_RejectsAbsurdMemory checks the extreme case: a hash
// that asks for four terabytes of memory has to be rejected before argon2
// allocates anything. Without the bound this test would take the process down
// with an out-of-memory kill, which is exactly the denial of service the
// bound prevents.
func TestComparePassword_RejectsAbsurdMemory(t *testing.T) {
	svc := crypto.NewCryptographyService()

	hash := "$argon2id$v=19$m=4000000000,t=3,p=2$YWFhYWFhYWFhYWFhYWFhYQ$YmJiYmJiYmJiYmJiYmJiQ"

	err := svc.ComparePassword(hash, "password")
	require.ErrorIs(t, err, crypto.ErrParamsOutOfRange,
		"ComparePassword() error = %v, want crypto.ErrParamsOutOfRange", err)
}

// TestComparePassword_RejectsOversizedHash checks that a PHC string that is
// far longer than any real hash is rejected before it is parsed.
func TestComparePassword_RejectsOversizedHash(t *testing.T) {
	svc := crypto.NewCryptographyService()

	hash := "$argon2id$v=19$m=65536,t=3,p=2$" + strings.Repeat("A", 2000) + "$YmJiYmJiYmJiYmJiYmJiQ"

	err := svc.ComparePassword(hash, "password")
	require.ErrorIs(t, err, crypto.ErrInvalidHashFormat,
		"ComparePassword() error = %v, want crypto.ErrInvalidHashFormat", err)
}

func TestComparePassword_ErrCorruptedSalt(t *testing.T) {
	svc := crypto.NewCryptographyService()

	// Valid PHC format but corrupted base64 salt (contains characters not in base64 alphabet)
	hash := "$argon2id$v=19$m=65536,t=3,p=2$!!!invalid_base64_char$YmJiYmJiYmJiYmJiYmJiQ"

	err := svc.ComparePassword(hash, "password")
	require.ErrorIs(t, err, crypto.ErrCorruptedSalt,
		"ComparePassword() error = %v, want crypto.ErrCorruptedSalt", err)
}

func TestComparePassword_ErrCorruptedHash(t *testing.T) {
	svc := crypto.NewCryptographyService()

	// Valid PHC format, valid salt (16 bytes, no padding), but corrupted base64 hash
	hash := "$argon2id$v=19$m=65536,t=3,p=2$YWFhYWFhYWFhYWFhYWFhYQ$!!!invalid_base64_char"

	err := svc.ComparePassword(hash, "password")
	require.ErrorIs(t, err, crypto.ErrCorruptedHash,
		"ComparePassword() error = %v, want crypto.ErrCorruptedHash", err)
}

func TestComparePassword_ErrIncorrectPassword(t *testing.T) {
	svc := crypto.NewCryptographyService()

	hash, err := svc.HashPassword("correctpassword")
	require.NoError(t, err, "HashPassword() error = %v", err)

	err = svc.ComparePassword(hash, "wrongpassword")
	require.ErrorIs(t, err, crypto.ErrIncorrectPassword,
		"ComparePassword() error = %v, want crypto.ErrIncorrectPassword", err)
}

func TestHashPassword_UniqueHashes(t *testing.T) {
	svc := crypto.NewCryptographyService()

	password := "samepassword"
	hash1, err := svc.HashPassword(password)
	require.NoError(t, err, "HashPassword() error = %v", err)

	hash2, err := svc.HashPassword(password)
	require.NoError(t, err, "HashPassword() error = %v", err)

	// Same password should produce different hashes due to random salt
	assert.NotEqual(t, hash1, hash2,
		"HashPassword() produced identical hashes for same password: %q", hash1)

	// But both should verify correctly
	assert.NoError(t, svc.ComparePassword(hash1, password),
		"ComparePassword() failed for hash1: %v", hash1)
	assert.NoError(t, svc.ComparePassword(hash2, password),
		"ComparePassword() failed for hash2: %v", hash2)
}
