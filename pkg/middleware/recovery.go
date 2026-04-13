package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// RecoveryConfig holds configuration for the recovery middleware.
type RecoveryConfig struct {
	// EnableStackTrace controls whether stack traces are logged on panic.
	EnableStackTrace bool

	// StackTraceInResponse controls whether stack traces are included in
	// the error response body (only recommended for development environments).
	StackTraceInResponse bool
}

// DefaultRecoveryConfig returns a RecoveryConfig with sensible defaults.
// Note: EnableStackTrace is kept true so panics are always visible in logs,
// which is useful for debugging even in production.
var DefaultRecoveryConfig = RecoveryConfig{
	EnableStackTrace:     true,
	StackTraceInResponse: false,
}

// RecoveryMiddleware returns a Fiber middleware that recovers from panics,
// logs the error and stack trace, and returns a 500 Internal Server Error
// response to the client instead of crashing the server.
func RecoveryMiddleware() fiber.Handler {
	return RecoveryMiddlewareWithConfig(DefaultRecoveryConfig)
}

// RecoveryMiddlewareWithConfig returns a recovery middleware with custom configuration.
func RecoveryMiddlewareWithConfig(cfg RecoveryConfig) fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				var panicErr error
				switch v := r.(type) {
				case error:
					panicErr = v
				default:
					panicErr = fmt.Errorf("%v", v)
				}

				logEvent := log.Error().
					Err(panicErr).
					Str("method", c.Method()).
					Str("path", c.Path()).
					Str("ip", c.IP())

				if cfg.EnableStackTrace {
					stack := debug.Stack()
					logEvent = logEvent.Bytes("stack", stack)
				}

				logEvent.Msg("recovered from panic")

				// Build the response body.
				// Always return a generic message to the client to avoid leaking
				// internal details unless StackTraceInResponse is explicitly enabled.
				responseBody := fiber.Map{
					"error": fiber.Map{
						"message": "internal server error",
						"type":    "server_error",
					},
				}

				if cfg.StackTraceInResponse {
					responseBody["error"] = fiber.Map{
						"message": panicErr.Error(),
						"type":    "server_error",
						"stack":   string(debug.Stack()),
					}
				}

				// Attempt to send a 500 response; ignore any secondary error.
				_ = c.Status(http.StatusInternalServerError).JSON(responseBody)
			}
		}()

		return c.Next()
	}
}
