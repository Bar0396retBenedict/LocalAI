package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// LoggerConfig defines configuration for the request logger middleware.
type LoggerConfig struct {
	// SkipPaths is a list of URL paths to skip logging for (e.g. health checks).
	SkipPaths []string

	// SlowRequestThreshold defines the duration above which a request is
	// considered slow and logged at warn level.
	SlowRequestThreshold time.Duration
}

// DefaultLoggerConfig provides sensible defaults for the logger middleware.
var DefaultLoggerConfig = LoggerConfig{
	SkipPaths:            []string{"/healthz", "/readyz", "/metrics"},
	SlowRequestThreshold: 5 * time.Second,
}

// RequestLoggerMiddleware returns a Fiber middleware that logs each HTTP request
// using zerolog. It attaches the request-id (if present) to every log entry.
func RequestLoggerMiddleware(cfg LoggerConfig) fiber.Handler {
	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		path := c.Path()

		// Skip logging for configured paths.
		if _, skip := skipSet[path]; skip {
			return c.Next()
		}

		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		status := c.Response().StatusCode()
		method := c.Method()
		ip := c.IP()
		reqID := GetRequestID(c)

		event := log.Info()
		if duration >= cfg.SlowRequestThreshold {
			event = log.Warn().Bool("slow_request", true)
		}
		if status >= 500 {
			event = log.Error()
		}

		event.
			Str("request_id", reqID).
			Str("method", method).
			Str("path", path).
			Str("ip", ip).
			Int("status", status).
			Dur("duration", duration).
			Int("bytes_out", len(c.Response().Body())).
			Msg("request")

		return err
	}
}

// NewRequestLoggerMiddleware returns a RequestLoggerMiddleware with default config.
func NewRequestLoggerMiddleware() fiber.Handler {
	return RequestLoggerMiddleware(DefaultLoggerConfig)
}
