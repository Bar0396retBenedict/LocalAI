# Response Streaming in LocalAI

This guide covers how to implement and work with streaming responses in LocalAI, including SSE (Server-Sent Events) for chat completions and token-by-token output.

## Overview

LocalAI supports streaming responses compatible with the OpenAI API. When a client sends `"stream": true`, the server sends incremental chunks using the `text/event-stream` content type.

## Core Streaming Pattern

```go
package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// StreamWriter wraps a fiber.Ctx and provides helpers for SSE output.
type StreamWriter struct {
	c      *fiber.Ctx
	writer *bufio.Writer
}

// NewStreamWriter sets the appropriate SS and returns a StreamWriter.
func NewStreamWriter(c *fiber.Ctx) *StreamWriter {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")
	return &StreamWriter{c: c}
}

// WriteChunk serializes a chunk and writes it as an SSE data line.
func (sw *StreamWriter) WriteChunk(chunk interface{}) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("failed to marshal chunk: %w", err)
	}
	_, err = fmt.Fprintf(sw.c.Response().BodyWriter(), "data: %s\n\n", data)
	return err
}

// WriteDone sends the terminal [DONE] SSE message.
func (sw *StreamWriter) WriteDone() error {
	_, err := fmt.Fprint(sw.c.Response().BodyWriter(), "data: [DONE]\n\n")
	return err
}
```

## Streaming Chat Completions

```go
// streamChatCompletion streams token chunks back to the client.
// It reads from a channel populated by the backend inference goroutine.
func streamChatCompletion(
	ctx context.Context,
	c *fiber.Ctx,
	modelName string,
	tokenCh <-chan string,
	errCh <-chan error,
) error {
	sw := NewStreamWriter(c)
	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer func() {
			if err := sw.WriteDone(); err != nil {
				log.Warn().Err(err).Msg("failed to write SSE [DONE]")
			}
			w.Flush()
		}()

		for {
			select {
			case <-ctx.Done():
				log.Debug().Str("model", modelName).Msg("stream context cancelled")
				return

			case err, ok := <-errCh:
				if !ok {
					return
				}
				log.Error().Err(err).Str("model", modelName).Msg("streaming error from backend")
				// Emit an error chunk so the client knows something went wrong.
				errChunk := map[string]interface{}{
					"error": map[string]string{
						"message": err.Error(),
						"type":    "server_error",
					},
				}
				if marshalErr := sw.WriteChunk(errChunk); marshalErr != nil {
					log.Warn().Err(marshalErr).Msg("failed to write error chunk")
				}
				return

			case token, ok := <-tokenCh:
				if !ok {
					// Channel closed — inference finished.
					return
				}
				chunk := ChatCompletionStreamChunk{
					ID:      chunkID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   modelName,
					Choices: []StreamChoice{
						{
							Index: 0,
							Delta: ChoiceDelta{Content: token},
						},
					},
				}
				if err := sw.WriteChunk(chunk); err != nil {
					log.Warn().Err(err).Msg("failed to write token chunk")
					return
				}
				w.Flush()
			}
		}
	})
}
```

## Chunk Data Structures

```go
// ChatCompletionStreamChunk mirrors the OpenAI streaming chunk format.
type ChatCompletionStreamChunk struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []StreamChoice  `json:"choices"`
}

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        ChoiceDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type ChoiceDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}
```

## Handler Integration

In your route handler, check `request.Stream` before choosing the response path:

```go
if req.Stream {
	return streamChatCompletion(c.Context(), c, req.Model, tokenCh, errCh)
}
// Otherwise build and return the full response.
```

## Key Points

- Always set `Transfer-Encoding: chunked` and `Content-Type: text/event-stream`.
- Flush after every chunk to avoid buffering delays.
- Respect context cancellation — stop streaming when the client disconnects.
- Send `data: [DONE]\n\n` as the final message.
- Emit structured error chunks rather than silently dropping errors mid-stream.
- Use a dedicated goroutine for inference and communicate via channels to keep the handler non-blocking.
