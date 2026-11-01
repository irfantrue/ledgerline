package middleware

import (
	"strings"

	"ledgerline/internal/shared/config"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

// NewCors creates a new Fiber CORS middleware based on the application configuration.
// CORS_ALLOWED_ORIGINS is a comma-separated list (spaces around items are ignored).
//
// An empty list means "*" with credentials disabled in development. In
// production an empty list closes the origins instead of opening them, because
// the cors middleware treats an empty list as "every origin". The
// configuration rejects an empty list in production, so this is the second
// line of defence.
func NewCors(cfg config.Config) fiber.Handler {
	origins := ParseOrigins(cfg.App.CorsAllowedOrigins)

	corsConfig := cors.Config{
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Timezone"},
		AllowMethods: []string{"GET", "POST", "HEAD", "PUT", "DELETE", "PATCH"},
	}

	switch {
	case len(origins) == 0 && cfg.App.Env == "production":
		// The function rejects every origin, so no cross-origin request is
		// answered with a CORS header.
		corsConfig.AllowOriginsFunc = func(string) bool { return false }
	case len(origins) == 0:
		corsConfig.AllowOrigins = []string{"*"}
	default:
		corsConfig.AllowOrigins = origins
	}

	// Credentials are only combined with an explicit list of origins. The
	// cors middleware panics when they are combined with a wildcard.
	corsConfig.AllowCredentials = len(origins) > 0 && !containsWildcard(origins)

	return cors.New(corsConfig)
}

// ParseOrigins splits a comma-separated origin list into its entries,
// trimming whitespace and dropping empty ones.
func ParseOrigins(raw string) []string {
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// containsWildcard reports whether the origin list allows every origin.
func containsWildcard(origins []string) bool {
	for _, origin := range origins {
		if origin == "*" {
			return true
		}
	}
	return false
}
