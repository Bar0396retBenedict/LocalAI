# Request Validation and Sanitization in LocalAI

This guide covers how to validate and sanitize incoming API requests in LocalAI to ensure correctness, security, and graceful error handling.

## Overview

LocalAI uses a combination of struct-level validation, custom validators, and sanitization helpers to protect backend model calls from malformed or malicious input. All validation logic lives close to the handler layer, before any model or backend code is invoked.

## Validation Libraries

LocalAI uses:
- `github.com/go-playground/validator/v10` for struct tag-based validation
- Custom helper functions for domain-specific rules (e.g., model name format, token limits)

## Struct Validation Example

```go
package api

import (
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// CompletionRequest represents a validated chat/completion request.
type CompletionRequest struct {
	Model       string    `json:"model" validate:"required,min=1,max=256"`
	Prompt      string    `json:"prompt" validate:"required,min=1,max=32768"`
	MaxTokens   int       `json:"max_tokens" validate:"omitempty,min=1,max=32768"`
	Temperature float64   `json:"temperature" validate:"omitempty,min=0,max=2"`
	TopP        float64   `json:"top_p" validate:"omitempty,min=0,max=1"`
	Stream      bool      `json:"stream"`
	Stop        []string  `json:"stop" validate:"omitempty,max=4,dive,min=1,max=64"`
}

// ValidateCompletionRequest parses and validates a CompletionRequest.
// Returns a human-readable error string on failure, or empty string on success.
func ValidateCompletionRequest(req *CompletionRequest) string {
	if err := validate.Struct(req); err != nil {
		return formatValidationErrors(err)
	}
	return ""
}

// formatValidationErrors converts validator.ValidationErrors into a readable message.
func formatValidationErrors(err error) string {
	var msgs []string
	for _, e := range err.(validator.ValidationErrors) {
		msgs = append(msgs, e.Field()+": "+e.Tag())
	}
	return strings.Join(msgs, "; ")
}
```

## Model Name Sanitization

Model names must be sanitized before being used in file paths or backend lookups to prevent path traversal attacks.

```go
import (
	"path/filepath"
	"strings"
	"regexp"
)

var safeModelName = regexp.MustCompile(`^[a-zA-Z0-9_\-\./:]+$`)

// SanitizeModelName ensures a model name is safe for use in file paths and backend lookups.
// Returns the cleaned name and a boolean indicating if it was valid.
func SanitizeModelName(name string) (string, bool) {
	// Remove any path traversal components
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if !safeModelName.MatchString(name) {
		return "", false
	}
	return name, true
}
```

## Handler Integration

Validation should happen at the top of every handler, before any processing:

```go
func completionHandler(c *fiber.Ctx) error {
	var req CompletionRequest
	if err := c.BodyParser(&req); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, "invalid JSON: "+err.Error())
	}

	if msg := ValidateCompletionRequest(&req); msg != "" {
		return errorResponse(c, fiber.StatusBadRequest, "validation error: "+msg)
	}

	cleanModel, ok := SanitizeModelName(req.Model)
	if !ok {
		return errorResponse(c, fiber.StatusBadRequest, "invalid model name")
	}
	req.Model = cleanModel

	// ... proceed with backend call
}
```

## Prompt Sanitization

For text prompts, LocalAI does **not** strip content by default (models handle arbitrary text), but does enforce length limits and rejects null bytes:

```go
// SanitizePrompt removes null bytes and enforces max length.
func SanitizePrompt(prompt string, maxLen int) (string, bool) {
	prompt = strings.ReplaceAll(prompt, "\x00", "")
	if len(prompt) > maxLen {
		return "", false
	}
	return prompt, true
}
```

## Testing Validation Logic

```go
func TestSanitizeModelName(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"llama-3-8b", true},
		{"../etc/passwd", false},
		{"model/v1:latest", true},
		{"model with spaces", false},
		{"", false},
	}
	for _, tc := range cases {
		_, ok := SanitizeModelName(tc.input)
		if ok != tc.want {
			t.Errorf("SanitizeModelName(%q) = %v, want %v", tc.input, ok, tc.want)
		}
	}
}
```

## Key Rules

1. **Always validate before processing** — never pass raw user input to backends.
2. **Sanitize model names** before any file system or map lookup.
3. **Use struct tags** for standard field constraints; use custom functions for domain rules.
4. **Return 400 Bad Request** for validation failures with a clear, non-leaking message.
5. **Do not log raw user prompts** at INFO level — use DEBUG and ensure log scrubbing is enabled in production.
6. **Enforce max token limits** at the API layer, not just in the backend, to prevent resource exhaustion.

## Related Guides

- `.agents/error-handling-and-logging.md` — how to return and log errors
- `.agents/rate-limiting-and-middleware.md` — middleware ordering (validation runs after auth, before backend)
- `.agents/api-endpoints-and-auth.md` — handler registration and auth flow
