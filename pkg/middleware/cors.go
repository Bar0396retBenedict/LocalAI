package middleware

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// CORSConfig holds configuration for the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is a comma-separated list of origins that are allowed.
	// Use "*" to allow all origins. Defaults to "*".
	AllowOrigins string

	// AllowMethods is a comma-separated list of HTTP methods allowed for CORS.
	// Defaults to common REST methods.
	AllowMethods string

	// AllowHeaders is a comma-separated list of request headers allowed.
	AllowHeaders string

	// ExposeHeaders is a comma-separated list of response headers exposed to the browser.
	ExposeHeaders string

	// AllowCredentials indicates whether the request can include user credentials.
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached.
	MaxAge int
}

// DefaultCORSConfig returns a CORSConfig with permissive defaults suitable
// for development and open API deployments.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID",
		ExposeHeaders:    "Content-Length,Content-Type",
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// CORSMiddleware returns a Fiber middleware handler that applies CORS headers
// based on the provided CORSConfig. When AllowCredentials is true, wildcard
// origins are replaced with the request's Origin header to satisfy the CORS
// specification (credentials + wildcard is forbidden).
func CORSMiddleware(cfg CORSConfig) fiber.Handler {
	allowOrigins := cfg.AllowOrigins
	if allowOrigins == "" {
		allowOrigins = "*"
	}

	allowMethods := cfg.AllowMethods
	if allowMethods == "" {
		allowMethods = "GET,POST,PUT,PATCH,DELETE,OPTIONS"
	}

	allowHeaders := cfg.AllowHeaders
	if allowHeaders == "" {
		allowHeaders = "Origin,Content-Type,Accept,Authorization"
	}

	corsCfg := cors.Config{
		AllowOrigins:     allowOrigins,
		AllowMethods:     allowMethods,
		AllowHeaders:     allowHeaders,
		ExposeHeaders:    cfg.ExposeHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge,
	}

	// When credentials are allowed we cannot use a wildcard origin; instead we
	// reflect the request origin dynamically.
	if cfg.AllowCredentials && strings.TrimSpace(allowOrigins) == "*" {
		corsCfg.AllowOriginsFunc = func(origin string) bool {
			return origin != ""
		}
		// Clear the static wildcard so the dynamic func takes precedence.
		corsCfg.AllowOrigins = ""
	}

	return cors.New(corsCfg)
}

// StrictCORSMiddleware returns a CORS middleware that only allows requests from
// the explicitly listed origins. Useful for production deployments where the
// set of trusted front-ends is known in advance.
func StrictCORSMiddleware(allowedOrigins []string) fiber.Handler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[strings.ToLower(strings.TrimSpace(o))] = struct{}{}
	}

	return cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			_, ok := originSet[strings.ToLower(origin)]
			return ok
		},
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID",
		ExposeHeaders:    "Content-Length,Content-Type",
		AllowCredentials: true,
		MaxAge:           http.StatusOK, // reuse constant — actual value 200 used as cache seconds
	})
}
