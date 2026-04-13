# Error Handling and Logging in LocalAI

This guide covers best practices for error handling and structured logging throughout the LocalAI codebase.

## Logging

LocalAI uses [zerolog](https://github.com/rs/zerolog) for structured, leveled logging.

### Import

```go
import "github.com/rs/zerolog/log"
```

### Log Levels

| Level | Usage |
|-------|-------|
| `log.Trace()` | Very verbose, internal state transitions |
| `log.Debug()` | Useful during development/debugging |
| `log.Info()` | Normal operational messages |
| `log.Warn()` | Something unexpected but recoverable |
| `log.Error()` | An error occurred, operation failed |
| `log.Fatal()` | Unrecoverable error, process will exit |

### Structured Fields

Always prefer structured fields over string interpolation:

```go
// Good
log.Info().
    Str("model", modelName).
    Str("backend", backendName).
    Int("tokens", tokenCount).
    Msg("inference completed")

// Avoid
log.Info().Msgf("inference completed for model %s using %s backend with %d tokens", modelName, backendName, tokenCount)
```

### Logging Errors

```go
if err != nil {
    log.Error().
        Err(err).
        Str("model", modelName).
        Msg("failed to load model")
    return fmt.Errorf("loading model %s: %w", modelName, err)
}
```

## Error Handling

### Wrapping Errors

Always wrap errors with context using `fmt.Errorf` and `%w`:

```go
if err := backend.Load(cfg); err != nil {
    return fmt.Errorf("backend %s load: %w", cfg.Backend, err)
}
```

### Sentinel Errors

Define sentinel errors for conditions callers may need to check:

```go
var (
    ErrModelNotFound   = errors.New("model not found")
    ErrBackendNotReady = errors.New("backend not ready")
    ErrContextExceeded = errors.New("context length exceeded")
)
```

Check with `errors.Is`:

```go
if errors.Is(err, ErrModelNotFound) {
    c.JSON(http.StatusNotFound, schema.ErrorResponse{Error: &schema.APIError{
        Message: err.Error(),
        Code:    http.StatusNotFound,
    }})
    return
}
```

### HTTP Error Responses

Use the shared error response schema for all API handlers:

```go
func errorResponse(c *fiber.Ctx, status int, msg string) error {
    return c.Status(status).JSON(schema.ErrorResponse{
        Error: &schema.APIError{
            Message: msg,
            Code:    status,
        },
    })
}
```

Common patterns:

```go
// 400 Bad Request
if req.Model == "" {
    return errorResponse(c, fiber.StatusBadRequest, "model is required")
}

// 404 Not Found
if _, err := ml.GetModel(req.Model); errors.Is(err, ErrModelNotFound) {
    return errorResponse(c, fiber.StatusNotFound, "model '"+req.Model+"' not found")
}

// 500 Internal Server Error
if err != nil {
    log.Error().Err(err).Str("model", req.Model).Msg("inference error")
    return errorResponse(c, fiber.StatusInternalServerError, "inference failed: "+err.Error())
}
```

## Panic Recovery

Backend goroutines should recover from panics to avoid crashing the whole server:

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Error().
                Interface("panic", r).
                Str("backend", backendName).
                Msg("recovered from panic in backend goroutine")
        }
    }()
    // ... backend work
}()
```

## Context Cancellation

Respect context cancellation in long-running operations:

```go
select {
case result := <-resultCh:
    return result, nil
case <-ctx.Done():
    log.Warn().
        Str("model", modelName).
        Msg("inference cancelled by client")
    return nil, ctx.Err()
}
```

## Do Not

- Do **not** swallow errors silently (`_ = someFunc()`)
- Do **not** use `log.Fatal` inside request handlers — return an error instead
- Do **not** log sensitive data (API keys, user prompts at Info level)
- Do **not** use `panic` outside of `init()` or truly unrecoverable startup failures
