package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// TimeoutConfig holds configuration for the request timeout middleware.
type TimeoutConfig struct {
	// Timeout is the maximum duration allowed for a single request.
	// Requests exceeding this duration will receive a 408 Request Timeout response.
	Timeout time.Duration

	// SkipPaths is a list of URL paths that should bypass timeout enforcement.
	// Useful for long-running endpoints like streaming completions or health checks.
	SkipPaths []string
}

// DefaultTimeoutConfig provides sensible defaults for the timeout middleware.
// Increased default timeout to 60s since 30s was too aggressive for slower
// local models (e.g. large GGUF files on CPU-only machines).
// Personal note: bumped to 120s because my Raspberry Pi 5 needs extra headroom
// when loading large quantized models for the first time.
var DefaultTimeoutConfig = TimeoutConfig{
	Timeout:   120 * time.Second,
	SkipPaths: []string{"/readyz", "/healthz"},
}

// TimeoutMiddleware returns a Fiber middleware that enforces a maximum request duration.
// If the handler does not complete within the configured timeout, the request is
// cancelled and a 408 status is returned to the client.
func TimeoutMiddleware(cfg TimeoutConfig) fiber.Handler {
	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		// Skip timeout enforcement for configured paths.
		if _, skip := skipSet[c.Path()]; skip {
			return c.Next()
		}

		// Derive a cancellable context from the request context.
		ctx, cancel := context.WithTimeout(c.Context(), cfg.Timeout)
		defer cancel()

		// Store the timeout context so downstream handlers can respect cancellation.
		c.SetUserContext(ctx)

		// Channel to capture the handler result.
		doneCh := make(chan error, 1)

		go func() {
			doneCh <- c.Next()
		}()

		select {
		case err := <-doneCh:
			return err
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				log.Warn().
					Str("method", c.Method()).
					Str("path", c.Path()).
					Dur("timeout", cfg.Timeout).
					Msg("request timed out")
				return c.Status(http.StatusRequestTimeout).JSON(fiber.Map{
					"error": fiber.Map{
						"message": "request timed out",
						"type":    "timeout_error",
						"code":    http.StatusRequestTimeout,
					},
				})
			}
			return ctx.Err()
		}
	}
}

// StreamingTimeoutMiddleware returns a middleware with an extended timeout suitable
// for streaming endpoints (e.g. /v1/chat/completions with stream=true).
// It applies a much longer deadline so that token-by-token responses are not
// prematurely cut off while still guarding against completely hung connections.
func StreamingTimeoutMiddleware(streamTimeout time.Duration) fiber.Handler {
	return TimeoutMiddleware(TimeoutConfig{
		Timeout:   streamTimeout,
		SkipPaths: DefaultTimeoutConfig.SkipPaths,
	})
}
