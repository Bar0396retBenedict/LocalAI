# Concurrent Request Handling in LocalAI

This guide covers patterns for handling concurrent requests safely, managing worker pools, and preventing resource exhaustion in LocalAI.

## Overview

LocalAI uses a combination of Go channels, mutexes, and semaphores to manage concurrent model inference requests. Each backend may have different concurrency constraints (e.g., llama.cpp is typically single-threaded per model instance).

## Worker Pool Pattern

Use a buffered channel as a semaphore to limit concurrent requests per model:

```go
package backend

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/rs/zerolog/log"
)

// ModelSemaphore limits concurrent inference requests for a single model instance.
type ModelSemaphore struct {
    ch      chan struct{}
    timeout time.Duration
}

// NewModelSemaphore creates a semaphore allowing up to maxConcurrent simultaneous requests.
func NewModelSemaphore(maxConcurrent int, timeout time.Duration) *ModelSemaphore {
    return &ModelSemaphore{
        ch:      make(chan struct{}, maxConcurrent),
        timeout: timeout,
    }
}

// Acquire blocks until a slot is available or the context is cancelled.
func (s *ModelSemaphore) Acquire(ctx context.Context) error {
    select {
    case s.ch <- struct{}{}:
        return nil
    case <-ctx.Done():
        return fmt.Errorf("request cancelled while waiting for model slot: %w", ctx.Err())
    case <-time.After(s.timeout):
        return fmt.Errorf("timed out waiting for available model slot after %s", s.timeout)
    }
}

// Release frees a slot back to the pool.
func (s *ModelSemaphore) Release() {
    <-s.ch
}
```

## Request Queue with Priority

For scenarios where some requests (e.g., streaming) should be prioritized:

```go
// PriorityQueue manages high and low priority inference jobs.
type PriorityQueue struct {
    high chan InferenceJob
    low  chan InferenceJob
    quit chan struct{}
}

type InferenceJob struct {
    ctx      context.Context
    payload  interface{}
    resultCh chan<- InferenceResult
}

type InferenceResult struct {
    Data  interface{}
    Error error
}

func NewPriorityQueue(highCap, lowCap int) *PriorityQueue {
    return &PriorityQueue{
        high: make(chan InferenceJob, highCap),
        low:  make(chan InferenceJob, lowCap),
        quit: make(chan struct{}),
    }
}

// Next returns the next job, preferring high-priority jobs.
func (pq *PriorityQueue) Next() (InferenceJob, bool) {
    select {
    case job := <-pq.high:
        return job, true
    default:
    }
    select {
    case job := <-pq.high:
        return job, true
    case job := <-pq.low:
        return job, true
    case <-pq.quit:
        return InferenceJob{}, false
    }
}
```

## Model Lock Manager

When a model must be accessed exclusively (e.g., during loading or fine-tuning):

```go
// ModelLockManager tracks per-model RW locks to allow concurrent reads
// but exclusive writes (e.g., model reload).
type ModelLockManager struct {
    mu    sync.Mutex
    locks map[string]*sync.RWMutex
}

func NewModelLockManager() *ModelLockManager {
    return &ModelLockManager{
        locks: make(map[string]*sync.RWMutex),
    }
}

func (m *ModelLockManager) getLock(modelName string) *sync.RWMutex {
    m.mu.Lock()
    defer m.mu.Unlock()
    if _, ok := m.locks[modelName]; !ok {
        m.locks[modelName] = &sync.RWMutex{}
    }
    return m.locks[modelName]
}

// RLock acquires a read lock for inference.
func (m *ModelLockManager) RLock(modelName string) {
    m.getLock(modelName).RLock()
}

// RUnlock releases the read lock.
func (m *ModelLockManager) RUnlock(modelName string) {
    m.getLock(modelName).RUnlock()
}

// Lock acquires an exclusive write lock (e.g., for model reload).
func (m *ModelLockManager) Lock(modelName string) {
    log.Debug().Str("model", modelName).Msg("acquiring exclusive model lock")
    m.getLock(modelName).Lock()
}

// Unlock releases the exclusive write lock.
func (m *ModelLockManager) Unlock(modelName string) {
    m.getLock(modelName).Unlock()
    log.Debug().Str("model", modelName).Msg("released exclusive model lock")
}
```

## Usage in Request Handlers

```go
func (h *Handler) Infer(ctx context.Context, modelName string, req InferenceRequest) (*InferenceResponse, error) {
    // Acquire shared read lock — allows concurrent inference
    h.lockManager.RLock(modelName)
    defer h.lockManager.RUnlock(modelName)

    // Acquire semaphore slot to cap concurrency
    if err := h.semaphores[modelName].Acquire(ctx); err != nil {
        return nil, err
    }
    defer h.semaphores[modelName].Release()

    return h.runInference(ctx, modelName, req)
}
```

## Best Practices

- **Always pass `context.Context`** to allow cancellation of queued requests.
- **Set reasonable semaphore timeouts** (e.g., 30s) to avoid indefinite blocking.
- **Use `RWMutex` for model access** — reads (inference) are concurrent, writes (reload) are exclusive.
- **Monitor queue depth** via metrics (see `observability-and-metrics.md`) to detect backpressure.
- **Avoid holding locks during I/O** — release before streaming responses.
- **Per-model semaphores** are preferred over a global one to avoid head-of-line blocking.

## Related Files

- `.agents/response-streaming.md` — streaming response patterns
- `.agents/context-and-cancellation.md` — context propagation
- `.agents/observability-and-metrics.md` — monitoring queue depth
- `.agents/rate-limiting-and-middleware.md` — upstream rate limiting
