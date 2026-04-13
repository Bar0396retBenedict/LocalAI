# Rate Limiting and Middleware in LocalAI

This document describes how to implement and configure rate limiting, authentication middleware, and request pipeline middleware in LocalAI.

## Middleware Architecture

LocalAI uses [Fiber](https://gofiber.io/) as its HTTP framework. Middleware is registered on the Fiber app or on specific route groups.

### Registering Middleware

Middleware is registered in `core/http/app.go` via the `App()` function:

```go
app := fiber.New(fiber.Config{
    BodyLimit: appConfig.UploadLimitMB * 1024 * 1024,
    ReadTimeout: time.Duration(appConfig.ReadTimeout) * time.Second,
})

// Global middleware
app.Use(recover.New())
app.Use(requestid.New())
app.Use(logger.New())
```

## Rate Limiting

### Built-in Fiber Rate Limiter

Use `github.com/gofiber/fiber/v2/middleware/limiter` for basic rate limiting:

```go
import "github.com/gofiber/fiber/v2/middleware/limiter"

func RateLimitMiddleware(appConfig *config.ApplicationConfig) fiber.Handler {
    if appConfig.RateLimitRequests == 0 {
        // Rate limiting disabled
        return func(c *fiber.Ctx) error {
            return c.Next()
        }
    }
    return limiter.New(limiter.Config{
        Max:        appConfig.RateLimitRequests,
        Expiration: time.Duration(appConfig.RateLimitWindowSec) * time.Second,
        KeyGenerator: func(c *fiber.Ctx) string {
            // Key by API key if present, otherwise by IP
            if key := c.Get("Authorization"); key != "" {
                return key
            }
            return c.IP()
        },
        LimitReached: func(c *fiber.Ctx) error {
            return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
                "error": fiber.Map{
                    "message": "Rate limit exceeded. Please slow down your requests.",
                    "type":    "rate_limit_error",
                    "code":    429,
                },
            })
        },
    })
}
```

### Applying Rate Limiting to Routes

```go
// Apply to all API routes
apiGroup := app.Group("/v1", RateLimitMiddleware(appConfig))
apiGroup.Post("/chat/completions", chatHandler)

// Apply stricter limits to expensive endpoints
app.Post("/v1/images/generations",
    limiter.New(limiter.Config{Max: 10, Expiration: time.Minute}),
    imageHandler,
)
```

## Authentication Middleware

### API Key Validation

See `.agents/api-endpoints-and-auth.md` for full auth details. The core middleware pattern:

```go
func AuthMiddleware(appConfig *config.ApplicationConfig) fiber.Handler {
    return func(c *fiber.Ctx) error {
        if len(appConfig.APIKeys) == 0 {
            return c.Next() // Auth disabled
        }
        token := extractBearerToken(c)
        for _, key := range appConfig.APIKeys {
            if subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1 {
                return c.Next()
            }
        }
        return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
            "error": fiber.Map{
                "message": "Invalid API key",
                "type":    "authentication_error",
                "code":    401,
            },
        })
    }
}

func extractBearerToken(c *fiber.Ctx) string {
    auth := c.Get("Authorization")
    if strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }
    return c.Query("api_key", "")
}
```

## Request Logging Middleware

```go
func RequestLoggerMiddleware() fiber.Handler {
    return func(c *fiber.Ctx) error {
        start := time.Now()
        err := c.Next()
        log.Debug().
            Str("method", c.Method()).
            Str("path", c.Path()).
            Int("status", c.Response().StatusCode()).
            Dur("latency", time.Since(start)).
            Str("ip", c.IP()).
            Msg("request")
        return err
    }
}
```

## CORS Middleware

```go
import "github.com/gofiber/fiber/v2/middleware/cors"

app.Use(cors.New(cors.Config{
    AllowOrigins: appConfig.CORSAllowOrigins, // e.g. "*" or "https://example.com"
    AllowHeaders: "Origin, Content-Type, Accept, Authorization",
    AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
}))
```

## Configuration Fields

Add to `core/config/application_config.go`:

```go
RateLimitRequests   int    // Max requests per window (0 = disabled)
RateLimitWindowSec  int    // Window size in seconds (default: 60)
CORSAllowOrigins    string // Allowed CORS origins
```

## Middleware Order

Order matters. Recommended registration order in `App()`:

1. `recover.New()` — panic recovery
2. `requestid.New()` — attach request IDs
3. `RequestLoggerMiddleware()` — log all requests
4. `cors.New(...)` — CORS headers
5. `RateLimitMiddleware(...)` — rate limiting
6. `AuthMiddleware(...)` — authentication
7. Route handlers

## Testing Middleware

```go
func TestRateLimitMiddleware(t *testing.T) {
    app := fiber.New()
    cfg := &config.ApplicationConfig{RateLimitRequests: 2, RateLimitWindowSec: 60}
    app.Use(RateLimitMiddleware(cfg))
    app.Get("/test", func(c *fiber.Ctx) error { return c.SendString("ok") })

    for i := 0; i < 2; i++ {
        resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil))
        assert.Equal(t, 200, resp.StatusCode)
    }
    resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil))
    assert.Equal(t, 429, resp.StatusCode)
}
```
