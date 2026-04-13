# Caching and Performance in LocalAI

This guide covers caching strategies, performance optimizations, and best practices for LocalAI backends and API handlers.

## Response Caching

LocalAI supports in-memory caching for repeated requests with identical parameters. Use the `ResponseCache` to avoid redundant model inference.

```go
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// CacheEntry holds a cached response with expiry metadata.
type CacheEntry struct {
	Value     interface{}
	CreatedAt time.Time
	TTL       time.Duration
}

// IsExpired returns true if the cache entry has exceeded its TTL.
func (e *CacheEntry) IsExpired() bool {
	if e.TTL == 0 {
		return false // no expiry
	}
	return time.Since(e.CreatedAt) > e.TTL
}

// ResponseCache is a thread-safe in-memory cache for API responses.
type ResponseCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	defaultTTL time.Duration
}

// NewResponseCache creates a new ResponseCache with a default TTL.
func NewResponseCache(defaultTTL time.Duration) *ResponseCache {
	return &ResponseCache{
		entries:    make(map[string]*CacheEntry),
		defaultTTL: defaultTTL,
	}
}

// CacheKey generates a deterministic cache key from a request object.
func CacheKey(req interface{}) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Get retrieves a value from the cache. Returns nil if not found or expired.
func (c *ResponseCache) Get(key string) interface{} {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || entry.IsExpired() {
		return nil
	}
	return entry.Value
}

// Set stores a value in the cache with the default TTL.
func (c *ResponseCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &CacheEntry{
		Value:     value,
		CreatedAt: time.Now(),
		TTL:       c.defaultTTL,
	}
}

// Invalidate removes a key from the cache.
func (c *ResponseCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Purge removes all expired entries. Call periodically to free memory.
func (c *ResponseCache) Purge() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for k, v := range c.entries {
		if v.IsExpired() {
			delete(c.entries, k)
			removed++
		}
	}
	return removed
}
```

## Cache Integration in Handlers

Wrap your completion handler with cache lookup before invoking the model:

```go
func cachedCompletionHandler(cache *ResponseCache, next fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Only cache non-streaming requests
		var req CompletionRequest
		if err := c.BodyParser(&req); err != nil || req.Stream {
			return next(c)
		}

		key, err := CacheKey(req)
		if err != nil {
			return next(c)
		}

		if cached := cache.Get(key); cached != nil {
			c.Set("X-Cache", "HIT")
			return c.JSON(cached)
		}

		// Store original response writer to intercept
		c.Set("X-Cache", "MISS")
		return next(c)
	}
}
```

## Model Warm-Up

Pre-load frequently used models at startup to reduce first-request latency:

```go
// WarmUpModels loads a list of models into memory at startup.
// Logs warnings for models that fail to load but does not block startup.
func WarmUpModels(loader *model.ModelLoader, models []string) {
	for _, name := range models {
		go func(m string) {
			log.Printf("[warmup] pre-loading model: %s", m)
			if _, err := loader.LoadModel(m); err != nil {
				log.Printf("[warmup] warning: failed to pre-load %s: %v", m, err)
			}
		}(name)
	}
}
```

## Performance Tips

### Batch Requests
- Group multiple single-token requests where possible.
- Use `n > 1` in completion requests to generate multiple completions in one pass.

### Memory Management
- Set `GOMAXPROCS` appropriately for your CPU count.
- Monitor goroutine counts with `/metrics` (see observability guide).
- Use `runtime.GC()` sparingly; prefer tuning `GOGC` env var.

### Connection Pooling
- LocalAI backends communicate over gRPC; connections are pooled automatically.
- Avoid creating new backend processes per request — reuse via `ModelLoader`.

### Profiling
Enable pprof endpoints in development builds:

```go
import _ "net/http/pprof"

func init() {
	if os.Getenv("LOCALAI_PPROF") == "true" {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}
}
```

Then profile with:
```bash
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Cache TTL Recommendations

| Request Type       | Recommended TTL |
|--------------------|-----------------|
| Embeddings         | 1 hour          |
| Completions        | 5 minutes       |
| Chat completions   | 2 minutes       |
| Model list         | 30 seconds      |
| Streaming          | Never cache     |

## Related Guides
- [Concurrent Request Handling](.agents/concurrent-request-handling.md)
- [Observability and Metrics](.agents/observability-and-metrics.md)
- [Context and Cancellation](.agents/context-and-cancellation.md)
