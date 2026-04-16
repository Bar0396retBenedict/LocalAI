package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader is the HTTP header used to propagate request IDs.
	RequestIDHeader = "X-Request-ID"

	// RequestIDKey is the key used to store the request ID in the Gin context.
	RequestIDKey = "request_id"
)

// requestIDContextKey is an unexported type for context keys in this package.
type requestIDContextKey struct{}

// generateRequestID creates a cryptographically random 16-byte hex string.
// Using 16 bytes gives us 128 bits of entropy, which is sufficient for
// uniqueness across distributed systems without being excessively long.
func generateRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RequestIDMiddleware ensures every request has a unique ID.
// It reads the X-Request-ID header if present; otherwise it generates a new one.
// The ID is stored in the Gin context under RequestIDKey and echoed back in
// the response header so clients can correlate logs and traces.
//
// Personal note: I've kept client-supplied IDs trusted for easier local debugging
// with tools like curl. For a production hardened setup, consider validating the
// format (e.g. enforce hex, max length) before accepting client values.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prefer a client-supplied ID so distributed traces stay correlated.
		// Note: we intentionally trust the client-supplied value here; if you
		// need to prevent ID spoofing, validate or reject client-provided IDs.
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			id, err := generateRequestID()
			if err != nil {
				// Fall back to a static sentinel so the request can still proceed.
				id = "unknown"
			}
			requestID = id
		}

		// Expose the ID on the Gin context for handlers and other middleware.
		c.Set(RequestIDKey, requestID)

		// Attach the ID to the underlying request context so it flows into
		// downstream service calls and backend goroutines.
		ctx := context.WithValue(c.Request.Context(), requestIDContextKey{}, requestID)
		c.Request = c.Request.WithContext(ctx)

		// Echo the ID back so callers can reference it in support requests.
		c.Header(RequestIDHeader, requestID)

		c.Next()
	}
}

// GetRequestID retrieves the request ID from a Gin context.
// Returns an empty string if no ID has been set.
func GetRequestID(c *gin.Context) string {
	if id, exists := c.Get(RequestIDKey); exists {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return ""
}

// GetRequestIDFromContext retrieves the request ID stored in a plain
// context.Context, as set by RequestIDMiddleware.
func GetRequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		return id
	}
	return ""
}

// RequestIDFromRequest is a convenience helper for non-Gin code paths that
// only have access to an *http.Request.
func RequestIDFromRequest(r *http.Request) string {
	return GetRequestIDFromContext(r.Context())
}
