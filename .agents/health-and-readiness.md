# Health and Readiness Checks in LocalAI

This guide covers implementing health, liveness, and readiness endpoints for LocalAI services, including backend health monitoring and graceful degradation patterns.

## Overview

LocalAI exposes standard health check endpoints that can be used by orchestration systems (Kubernetes, Docker Swarm, load balancers) to determine service availability.

## Health Check Endpoints

### Basic Health Handler

```go
package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// HealthStatus represents the overall health of the service
type HealthStatus struct {
	Status    string                    `json:"status"`              // "ok", "degraded", "unavailable"
	Timestamp time.Time                 `json:"timestamp"`
	Uptime    string                    `json:"uptime"`
	Checks    map[string]ComponentCheck `json:"checks,omitempty"`
}

// ComponentCheck represents the health of a single component
type ComponentCheck struct {
	Status  string `json:"status"`            // "healthy", "unhealthy", "unknown"
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// HealthChecker manages component health checks
type HealthChecker struct {
	mu         sync.RWMutex
	checks     map[string]CheckFunc
	startTime  time.Time
}

// CheckFunc is a function that returns a ComponentCheck result
type CheckFunc func() ComponentCheck

// NewHealthChecker creates a new HealthChecker instance
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks:    make(map[string]CheckFunc),
		startTime: time.Now(),
	}
}

// Register adds a named health check function
func (h *HealthChecker) Register(name string, fn CheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = fn
}

// RunAll executes all registered checks and returns a HealthStatus
func (h *HealthChecker) RunAll() HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]ComponentCheck)
	overall := "ok"

	for name, fn := range h.checks {
		start := time.Now()
		result := fn()
		result.Latency = time.Since(start).String()
		results[name] = result

		if result.Status == "unhealthy" {
			overall = "degraded"
		}
	}

	return HealthStatus{
		Status:    overall,
		Timestamp: time.Now().UTC(),
		Uptime:    time.Since(h.startTime).Round(time.Second).String(),
		Checks:    results,
	}
}
```

## Fiber Route Registration

```go
// RegisterHealthRoutes attaches health endpoints to the Fiber app
func RegisterHealthRoutes(app *fiber.App, checker *HealthChecker) {
	// Liveness: is the process alive?
	app.Get("/healthz", livenessHandler())

	// Readiness: is the service ready to accept traffic?
	app.Get("/readyz", readinessHandler(checker))

	// Full health report with component details
	app.Get("/health", fullHealthHandler(checker))
}

func livenessHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	}
}

func readinessHandler(checker *HealthChecker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		status := checker.RunAll()
		httpStatus := http.StatusOK
		if status.Status != "ok" {
			httpStatus = http.StatusServiceUnavailable
		}
		return c.Status(httpStatus).JSON(fiber.Map{
			"status": status.Status,
			"uptime": status.Uptime,
		})
	}
}

func fullHealthHandler(checker *HealthChecker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		status := checker.RunAll()
		httpStatus := http.StatusOK
		if status.Status == "unavailable" {
			httpStatus = http.StatusServiceUnavailable
		}
		return c.Status(httpStatus).JSON(status)
	}
}
```

## Backend Health Check Example

```go
// ModelBackendCheck returns a CheckFunc for a loaded model backend
func ModelBackendCheck(backendName string, pingFn func() error) CheckFunc {
	return func() ComponentCheck {
		if err := pingFn(); err != nil {
			log.Warn().Str("backend", backendName).Err(err).Msg("backend health check failed")
			return ComponentCheck{
				Status:  "unhealthy",
				Message: err.Error(),
			}
		}
		return ComponentCheck{Status: "healthy"}
	}
}
```

## Kubernetes Probe Configuration

```yaml
# In your Kubernetes Deployment spec:
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 15
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 20
  periodSeconds: 10
  failureThreshold: 5
```

## Integration with Metrics

Health check results can be exported as Prometheus gauges:

```go
// In your metrics init (see observability-and-metrics.md):
var componentHealthGauge = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "localai_component_healthy",
		Help: "1 if the component is healthy, 0 otherwise",
	},
	[]string{"component"},
)

// Call this after RunAll() to sync metrics:
func SyncHealthMetrics(status HealthStatus) {
	for name, check := range status.Checks {
		val := 0.0
		if check.Status == "healthy" {
			val = 1.0
		}
		componentHealthGauge.WithLabelValues(name).Set(val)
	}
}
```

## Best Practices

- **Liveness** (`/healthz`): Always returns 200 if the process is running. Never include slow checks here — a failed liveness probe causes a pod restart.
- **Readiness** (`/readyz`): Returns 503 if any critical component is unhealthy. Used to temporarily remove a pod from load balancer rotation.
- **Full health** (`/health`): Detailed report for dashboards and debugging. May include non-critical component status.
- Keep check functions **fast** (< 100ms). Use cached results for expensive checks with a TTL.
- Register backend checks **after** the model is loaded, not at startup.
- Use structured logging (`zerolog`) inside check functions for consistency with the rest of LocalAI.
