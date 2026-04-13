# Context and Cancellation Patterns in LocalAI

This guide covers how to properly use Go's `context.Context` for request lifecycle management, cancellation propagation, and timeout handling across LocalAI's backends and API handlers.

## Core Principles

- Every long-running operation MUST accept a `context.Context` as its first argument
- Never store contexts in structs; pass them explicitly through call chains
- Always respect context cancellation in inference loops and streaming responses
- Use `context.WithTimeout` for backend calls with configurable deadlines

## Request Context Flow

```
HTTP Request
    └── Fiber ctx  →  context.Context (via c.UserContext())
            └── Backend gRPC call (with deadline)
                    └── Inference loop (checks ctx.Done())
                            └── Stream response (cancelled on disconnect)
```

## Creating Contexts with Timeouts

```go
// In an API handler, derive a context with a per-request timeout
func (h *Handler) Completions(c *fiber.Ctx) error {
    // Pull the base context from the HTTP request lifecycle
    baseCtx := c.UserContext()

    // Apply a configurable inference timeout (default: 10 minutes)
    timeout := h.config.InferenceTimeout
    if timeout == 0 {
        timeout = 10 * time.Minute
    }

    ctx, cancel := context.WithTimeout(baseCtx, timeout)
    defer cancel()

    result, err := h.backend.Infer(ctx, request)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            return fiber.NewError(fiber.StatusGatewayTimeout, "inference timeout exceeded")
        }
        if errors.Is(err, context.Canceled) {
            // Client disconnected — log but don't return an error to the (gone) client
            log.Debug().Msg("client disconnected before inference completed")
            return nil
        }
        return errorResponse(c, err)
    }
    return c.JSON(result)
}
```

## Checking Cancellation in Inference Loops

Backend implementations that run token generation loops must poll `ctx.Done()`:

```go
func (b *LlamaCppBackend) GenerateTokens(ctx context.Context, prompt string, cb TokenCallback) error {
    for {
        // Check for cancellation before each token generation step
        select {
        case <-ctx.Done():
            log.Debug().Err(ctx.Err()).Msg("token generation cancelled")
            return ctx.Err()
        default:
        }

        token, done, err := b.model.NextToken()
        if err != nil {
            return fmt.Errorf("token generation error: %w", err)
        }

        if err := cb(token); err != nil {
            return fmt.Errorf("token callback error: %w", err)
        }

        if done {
            return nil
        }
    }
}
```

## Streaming Responses with Context

For SSE (Server-Sent Events) streaming, wire the Fiber context's done channel:

```go
func streamCompletion(c *fiber.Ctx, ctx context.Context, tokenCh <-chan string) error {
    c.Set("Content-Type", "text/event-stream")
    c.Set("Cache-Control", "no-cache")
    c.Set("Connection", "keep-alive")

    c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
        for {
            select {
            case <-ctx.Done():
                // Timeout or client disconnect
                return
            case token, ok := <-tokenCh:
                if !ok {
                    // Stream finished — send terminal SSE event
                    fmt.Fprintf(w, "data: [DONE]\n\n")
                    w.Flush()
                    return
                }
                chunk := buildSSEChunk(token)
                fmt.Fprintf(w, "data: %s\n\n", chunk)
                w.Flush()
            }
        }
    })
    return nil
}
```

## gRPC Backend Context Propagation

When calling gRPC backends, always pass the context so deadlines and cancellation signals are forwarded:

```go
func (c *BackendClient) Predict(ctx context.Context, req *pb.PredictRequest) (*pb.Reply, error) {
    // gRPC automatically converts ctx deadline → gRPC deadline header
    resp, err := c.grpcClient.Predict(ctx, req)
    if err != nil {
        st, _ := status.FromError(err)
        switch st.Code() {
        case codes.DeadlineExceeded:
            return nil, fmt.Errorf("%w: gRPC backend deadline exceeded", context.DeadlineExceeded)
        case codes.Canceled:
            return nil, fmt.Errorf("%w: gRPC call cancelled", context.Canceled)
        }
        return nil, fmt.Errorf("gRPC predict error: %w", err)
    }
    return resp, nil
}
```

## Context Values — What to Store

Use `context.WithValue` sparingly. Accepted uses in LocalAI:

| Key | Type | Purpose |
|-----|------|---------|
| `requestIDKey` | `string` | Trace/correlation ID for logging |
| `modelNameKey` | `string` | Active model for the request |
| `authClaimsKey` | `*Claims` | Parsed JWT claims from middleware |

Never store mutable state, database connections, or large objects in context values.

```go
type contextKey string

const (
    requestIDKey contextKey = "requestID"
    modelNameKey contextKey = "modelName"
)

func WithRequestID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, requestIDKey, id)
}

func RequestIDFromContext(ctx context.Context) string {
    id, _ := ctx.Value(requestIDKey).(string)
    return id
}
```

## Testing Context Cancellation

```go
func TestInferenceCancelledOnTimeout(t *testing.T) {
    backend := newTestBackend(t)

    // Create a context that times out almost immediately
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
    defer cancel()

    time.Sleep(5 * time.Millisecond) // ensure deadline is already exceeded

    _, err := backend.Infer(ctx, testRequest())
    require.ErrorIs(t, err, context.DeadlineExceeded)
}
```

## Common Mistakes to Avoid

- **Don't** use `context.Background()` inside handler code — always propagate the request context
- **Don't** ignore `ctx.Err()` after a `select` on `ctx.Done()`
- **Don't** call `cancel()` only in error paths — always `defer cancel()` immediately after `WithTimeout`/`WithCancel`
- **Don't** wrap `context.Canceled` as an unexpected error — it is a normal shutdown signal
