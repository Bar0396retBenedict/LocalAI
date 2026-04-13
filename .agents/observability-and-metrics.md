# Observability and Metrics in LocalAI

This guide covers how to instrument code, expose metrics, and use tracing in LocalAI.

## Overview

LocalAI uses [Prometheus](https://prometheus.io/) for metrics and supports OpenTelemetry tracing.
Metrics are exposed at `/metrics` by default when enabled.

## Enabling Metrics

Set the following environment variable or CLI flag:

```bash
LOCALAI_METRICS=true
# or
./local-ai --metrics
```

The metrics endpoint listens on the same port as the API by default, or you can configure a separate port:

```bash
LOCALAI_METRICS_PORT=9090
```

## Available Metrics

| Metric Name | Type | Description |
|---|---|---|
| `localai_inference_duration_seconds` | Histogram | Time spent on inference per model |
| `localai_inference_requests_total` | Counter | Total number of inference requests |
| `localai_model_load_duration_seconds` | Histogram | Time to load a model into memory |
| `localai_active_models` | Gauge | Number of currently loaded models |
| `localai_token_throughput` | Gauge | Tokens generated per second |
| `localai_request_errors_total` | Counter | Total number of failed requests |

## Adding a New Metric

Metrics are defined in `pkg/metrics/metrics.go`. To add a new metric:

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var MyNewCounter = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "localai_my_feature_total",
        Help: "Total number of my feature invocations.",
    },
    []string{"model", "status"},
)

func init() {
    prometheus.MustRegister(MyNewCounter)
}
```

Then in your handler or backend code:

```go
metrics.MyNewCounter.WithLabelValues(modelName, "success").Inc()
```

## Tracing with OpenTelemetry

LocalAI supports optional OpenTelemetry tracing. Enable it via:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SERVICE_NAME=localai
```

### Creating a Span in Your Code

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
)

func MyTracedFunction(ctx context.Context, modelName string) error {
    tracer := otel.Tracer("localai")
    ctx, span := tracer.Start(ctx, "MyTracedFunction")
    defer span.End()

    span.SetAttributes(attribute.String("model", modelName))

    // ... your logic here
    return nil
}
```

## Structured Logging

LocalAI uses [logrus](https://github.com/sirupsen/logrus) for structured logging.

```go
import "github.com/rs/zerolog/log"

// Info level
log.Info().Str("model", modelName).Msg("Starting inference")

// Error with context
log.Error().Err(err).Str("model", modelName).Msg("Inference failed")

// Debug (only visible when LOG_LEVEL=debug)
log.Debug().Interface("config", cfg).Msg("Loaded model config")
```

Set the log level:

```bash
LOG_LEVEL=debug  # trace, debug, info, warn, error
```

## Health Endpoints

LocalAI exposes health check endpoints:

- `GET /healthz` — Returns `200 OK` if the service is running.
- `GET /readyz` — Returns `200 OK` if models are loaded and ready to serve.

Use these for Kubernetes liveness and readiness probes:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
```

## Grafana Dashboard

A sample Grafana dashboard JSON is available in `extras/grafana/localai-dashboard.json`.
Import it into your Grafana instance and point the data source to your Prometheus server.

## Tips

- Use histograms (not summaries) for latency metrics — they aggregate better across instances.
- Always add a `model` label to metrics so you can filter per-model in dashboards.
- Avoid high-cardinality labels (e.g., do not use request IDs as label values).
- Wrap expensive metric collection in `if log.Debug().Enabled()` to avoid overhead in production.
