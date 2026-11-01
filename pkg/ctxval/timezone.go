package ctxval

import (
	"context"
	"time"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

// timezoneKey is the context key used to store timezone information.
const timezoneKey contextKey = "user_timezone"

// GetTimezone retrieves the timezone location from the context.
// It returns time.UTC if the timezone is not set in the context.
func GetTimezone(ctx context.Context) *time.Location {
	if loc, ok := ctx.Value(timezoneKey).(*time.Location); ok {
		return loc
	}
	return time.UTC
}

// SetTimezone stores a timezone location into the given context and returns the new context.
func SetTimezone(ctx context.Context, loc *time.Location) context.Context {
	return context.WithValue(ctx, timezoneKey, loc)
}
