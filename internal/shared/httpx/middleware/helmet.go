package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
)

// NewHelmet creates a new Fiber helmet middleware to set various HTTP security headers.
func NewHelmet() fiber.Handler {
	return helmet.New(helmet.Config{
		// X-XSS-Protection: legacy XSS auditor is deprecated and can introduce issues; "0" disables it (rely on CSP)
		XSSProtection: "0",

		// X-Content-Type-Options: Prevents the browser from guessing the file type (MIME sniffing)
		ContentTypeNosniff: "nosniff",

		// X-Frame-Options: Prevents your API/Web from being embedded in another site's <iframe> (prevents Clickjacking)
		// Value "DENY" (block all) or "SAMEORIGIN" (allow embedding only by the same domain)
		XFrameOptions: "DENY",

		// Strict-Transport-Security (HSTS): Forces the browser to always use HTTPS (1 year = 31,536,000 seconds)
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
		HSTSPreloadEnabled:    true,

		// Referrer-Policy: Controls how much URL origin information is sent when the user navigates between pages.
		ReferrerPolicy: "strict-origin-when-cross-origin",
	})
}
