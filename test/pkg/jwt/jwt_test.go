package jwttest

import (
	"errors"
	"testing"
	"time"

	"ledgerline/pkg/jwt"

	gjwt "github.com/golang-jwt/jwt/v5"
)

const userOne = "user-1"

var testSecret = []byte("a-test-secret-that-is-long-enough")

func TestGenerateToken(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 24)

	res, err := svc.GenerateToken(jwt.Payload{UserID: userOne, TokenVersion: 3})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if res.Token == "" {
		t.Fatal("GenerateToken() returned an empty token")
	}
	if res.ExpiresAt.IsZero() {
		t.Fatal("GenerateToken() returned a zero expiry")
	}

	// The expiry must follow the configured duration.
	want := time.Now().Add(24 * time.Hour)
	if diff := res.ExpiresAt.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("ExpiresAt = %s, want about %s", res.ExpiresAt, want)
	}
}

func TestGenerateAndValidateRoundTrip(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	payload := jwt.Payload{UserID: "user-42", TokenVersion: 7}

	res, err := svc.GenerateToken(payload)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	got, err := svc.ValidateToken(res.Token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if got.UserID != payload.UserID {
		t.Errorf("UserID = %q, want %q", got.UserID, payload.UserID)
	}
	if got.TokenVersion != payload.TokenVersion {
		t.Errorf("TokenVersion = %d, want %d", got.TokenVersion, payload.TokenVersion)
	}
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
			if err == nil {
				t.Fatal("ValidateToken() error = nil, want ErrTokenInvalid")
			}
			if !errors.Is(err, jwt.ErrTokenInvalid) {
				t.Errorf("ValidateToken() error = %v, want ErrTokenInvalid", err)
			}
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
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	if _, err := svc.ValidateToken(tokenString); err == nil {
		t.Fatal("ValidateToken() error = nil, want ErrTokenInvalid for an expired token")
	}
}

// TestGenerateToken_RejectsNonPositiveExpiry checks that a service with a
// non-positive expiry refuses to issue a token instead of handing out one that
// is expired the moment it is created.
func TestGenerateToken_RejectsNonPositiveExpiry(t *testing.T) {
	for _, hours := range []int{0, -1} {
		svc := jwt.NewJWTService(testSecret, hours)

		_, err := svc.GenerateToken(jwt.Payload{UserID: userOne})
		if !errors.Is(err, jwt.ErrInvalidExpiry) {
			t.Errorf("GenerateToken() error = %v, want ErrInvalidExpiry for %d hours", err, hours)
		}
	}
}

// TestValidateToken_OtherHMACAlgorithm checks that only HS256 is accepted. A
// token signed with HS384 is an HMAC method too, so the check cannot be a
// type assertion on the signing method.
func TestValidateToken_OtherHMACAlgorithm(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	other := gjwt.NewWithClaims(gjwt.SigningMethodHS384, gjwt.MapClaims{"user_id": userOne})
	tokenString, err := other.SignedString(testSecret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	if _, err := svc.ValidateToken(tokenString); !errors.Is(err, jwt.ErrInvalidAlgorithm) {
		t.Errorf("ValidateToken() error = %v, want ErrInvalidAlgorithm", err)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	issuer := jwt.NewJWTService(testSecret, 1)
	verifier := jwt.NewJWTService([]byte("a-different-secret-that-is-long-enough"), 1)

	res, err := issuer.GenerateToken(jwt.Payload{UserID: userOne})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if _, err := verifier.ValidateToken(res.Token); err == nil {
		t.Fatal("ValidateToken() error = nil, want ErrTokenInvalid for a foreign signature")
	}
}

// TestValidateToken_NonHMACAlgorithm covers the algorithm check: a token
// signed with "none" must never be accepted.
func TestValidateToken_NonHMACAlgorithm(t *testing.T) {
	svc := jwt.NewJWTService(testSecret, 1)

	unsigned := gjwt.NewWithClaims(gjwt.SigningMethodNone, gjwt.MapClaims{"user_id": "attacker"})
	tokenString, err := unsigned.SignedString(gjwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	_, err = svc.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("ValidateToken() error = nil, want ErrInvalidAlgorithm")
	}
	if !errors.Is(err, jwt.ErrInvalidAlgorithm) {
		t.Errorf("ValidateToken() error = %v, want ErrInvalidAlgorithm", err)
	}
}
