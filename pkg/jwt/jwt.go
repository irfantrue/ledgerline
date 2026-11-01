package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Sentinel errors for JWT operations.
var (
	ErrInvalidAlgorithm = errors.New("token signing algorithm is not HMAC-SHA256")
	ErrTokenInvalid     = errors.New("token is invalid or has expired")
	ErrInvalidExpiry    = errors.New("token expiry must be a positive number of hours")
)

// CustomClaims combines standard JWT registered claims with custom payload.
type CustomClaims struct {
	Payload
	jwt.RegisteredClaims
}

// Ensure Service implementation at compile time.
var _ Service = (*jwtService)(nil)

// jwtService implements Service using HMAC-SHA256.
type jwtService struct {
	secretKey []byte
	expireHr  int
}

// NewJWTService creates a new JWT service with the given secret key and expiration duration.
func NewJWTService(secretKey []byte, expireHours int) Service {
	return &jwtService{
		secretKey: secretKey,
		expireHr:  expireHours,
	}
}

// GenerateToken creates a new JWT token containing the provided payload.
// The token is signed using HMAC-SHA256 and expires after the configured duration.
// It returns ErrInvalidExpiry when the configured duration is not positive,
// because such a token would be expired the moment it is created.
func (j *jwtService) GenerateToken(payload Payload) (*Response, error) {
	if j.expireHr <= 0 {
		return nil, ErrInvalidExpiry
	}

	expirationTime := time.Now().Add(time.Duration(j.expireHr) * time.Hour)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomClaims{
		Payload:   payload,
		ExpiresAt: jwt.NewNumericDate(expirationTime),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})

	tokenString, err := token.SignedString(j.secretKey)
	if err != nil {
		return nil, err
	}

	return &Response{
		Token:     tokenString,
		ExpiresAt: expirationTime,
	}, nil
}

// ValidateToken parses and validates a JWT token string.
// Returns the payload if valid, or ErrTokenInvalid if validation fails.
// Returns ErrInvalidAlgorithm if the token does not use HMAC-SHA256.
func (j *jwtService) ValidateToken(tokenString string) (*Payload, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (any, error) {
		// Only HS256 is accepted. Comparing the algorithm name rejects the
		// other HMAC variants and the "none" algorithm, which would let a
		// caller pick the signature itself.
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, ErrInvalidAlgorithm
		}
		return j.secretKey, nil
	})
	if err != nil {
		// The key function is where the algorithm is checked, so a token
		// signed with another algorithm ends up here. Report that case with
		// its own error instead of the generic one, otherwise the sentinel
		// is never reached by a caller.
		if errors.Is(err, ErrInvalidAlgorithm) {
			return nil, ErrInvalidAlgorithm
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*CustomClaims)
	if ok && token.Valid {
		return &claims.Payload, nil
	}

	return nil, ErrTokenInvalid
}
