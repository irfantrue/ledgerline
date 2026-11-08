package jwttest

import (
	"testing"
	"time"

	"ledgerline/pkg/jwt"

	gjwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const userOne = "user-1"

var testSecret = []byte("a-test-secret-that-is-long-enough")

func TestGenerateToken(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 24)

	res, err := svc.GenerateToken(jwt.Payload{UserID: userOne, TokenVersion: 3})
	require.NoError(t, err, "GenerateToken() error = %v", err)
	require.NotEmpty(t, res.Token, "GenerateToken() returned an empty token")
	require.False(t, res.ExpiresAt.IsZero(), "GenerateToken() returned a zero expiry")

	// The expiry must follow the configured duration.
	want := time.Now().Add(24 * time.Hour)
	assert.WithinDuration(t, want, res.ExpiresAt, time.Minute,
		"ExpiresAt = %s, want about %s", res.ExpiresAt, want)
}

func TestGenerateAndValidateRoundTrip(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	payload := jwt.Payload{UserID: "user-42", TokenVersion: 7}

	res, err := svc.GenerateToken(payload)
	require.NoError(t, err, "GenerateToken() error = %v", err)

	got, err := svc.ValidateToken(res.Token)
	require.NoError(t, err, "ValidateToken() error = %v", err)
	assert.Equal(t, payload.UserID, got.UserID, "UserID = %q, want %q", got.UserID, payload.UserID)
	assert.Equal(t, payload.TokenVersion, got.TokenVersion,
		"TokenVersion = %d, want %d", got.TokenVersion, payload.TokenVersion)
}

func TestValidateToken_Invalid(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	tests := []struct {
		name  string
		token string
	}{
		{name: "empty token", token: ""},
		{name: "garbage", token: "not-a-token"},
		//nolint:gosec // G101: a truncated token, not a credential
		{name: "truncated", token: "eyJhbGciOiJIUzI1NiJ9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ValidateToken(tt.token)
			require.Error(t, err, "ValidateToken() error = nil, want ErrTokenInvalid")
			require.ErrorIs(t, err, jwt.ErrTokenInvalid,
				"ValidateToken() error = %v, want ErrTokenInvalid", err)
		})
	}
}

func TestValidateToken_Expired(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	// The token is crafted directly: the service refuses to issue a token
	// with a non-positive expiry, so an expired one has to come from
	// somewhere else.
	expired := gjwt.NewWithClaims(gjwt.SigningMethodHS256, gjwt.MapClaims{
		"user_id": userOne,
		"exp":     time.Now().Add(-time.Hour).Unix(),
	})
	tokenString, err := expired.SignedString(testSecret)
	require.NoError(t, err, "SignedString() error = %v", err)

	_, err = svc.ValidateToken(tokenString)
	require.Error(t, err, "ValidateToken() error = nil, want ErrTokenInvalid for an expired token")
}

// TestGenerateToken_RejectsNonPositiveExpiry checks that a service with a
// non-positive expiry refuses to issue a token instead of handing out one that
// is expired the moment it is created.
func TestGenerateToken_RejectsNonPositiveExpiry(t *testing.T) {
	for _, hours := range []int{0, -1} {
		svc := jwt.NewJWTService(testSecret, hours)

		_, err := svc.GenerateToken(jwt.Payload{UserID: userOne})
		require.ErrorIs(t, err, jwt.ErrInvalidExpiry,
			"GenerateToken() error = %v, want ErrInvalidExpiry for %d hours", err, hours)
	}
}

// TestValidateToken_OtherHMACAlgorithm checks that only HS256 is accepted. A
// token signed with HS384 is an HMAC method too, so the check cannot be a
// type assertion on the signing method.
func TestValidateToken_OtherHMACAlgorithm(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	other := gjwt.NewWithClaims(gjwt.SigningMethodHS384, gjwt.MapClaims{"user_id": userOne})
	tokenString, err := other.SignedString(testSecret)
	require.NoError(t, err, "SignedString() error = %v", err)

	_, err = svc.ValidateToken(tokenString)
	require.ErrorIs(t, err, jwt.ErrInvalidAlgorithm,
		"ValidateToken() error = %v, want ErrInvalidAlgorithm", err)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	issuer := jwt.NewJWTService(testSecret, 1)
	verifier := jwt.NewJWTService([]byte("a-different-secret-that-is-long-enough"), 1)

	res, err := issuer.GenerateToken(jwt.Payload{UserID: userOne})
	require.NoError(t, err, "GenerateToken() error = %v", err)

	_, err = verifier.ValidateToken(res.Token)
	require.Error(t, err, "ValidateToken() error = nil, want ErrTokenInvalid for a foreign signature")
}

// TestValidateToken_NonHMACAlgorithm covers the algorithm check: a token
// signed with "none" must never be accepted.
func TestValidateToken_NonHMACAlgorithm(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	unsigned := gjwt.NewWithClaims(gjwt.SigningMethodNone, gjwt.MapClaims{"user_id": "attacker"})
	tokenString, err := unsigned.SignedString(gjwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err, "SignedString() error = %v", err)

	_, err = svc.ValidateToken(tokenString)
	require.Error(t, err, "ValidateToken() error = nil, want ErrInvalidAlgorithm")
	require.ErrorIs(t, err, jwt.ErrInvalidAlgorithm,
		"ValidateToken() error = %v, want ErrInvalidAlgorithm", err)
}
