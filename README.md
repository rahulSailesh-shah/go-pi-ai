# go-pi-ai

> A Go implementation inspired by [pi-mono](https://github.com/badlogic/pi-mono) by [Mario Zechner](https://github.com/badlogic).

A Go SDK for OpenAI-compatible chat completions with streaming and tool calling support. Works with OpenAI, NVIDIA, and any API that follows the OpenAI chat completions format.

## Installation

```bash
go get github.com/rahulSailesh-shah/go-pi-ai
```

Module path: `github.com/rahulSailesh-shah/go-pi-ai`

Import the root package as:

```go
import gopiai "github.com/rahulSailesh-shah/go-pi-ai"
```

Import the OpenAI provider as:

```go
import "github.com/rahulSailesh-shah/go-pi-ai/openai"
```

## Architecture

The SDK has two packages:

- **`gopiai`** (root) -- All types, interfaces, the `Client`, and the `Stream` iterator. This is what your application code depends on.
- **`gopiai/openai`** -- The OpenAI-compatible provider implementation. Talks to any OpenAI-compatible API.

The core abstraction is the `Provider` interface:

```go
type Provider interface {
    Complete(ctx context.Context, req Request) (AssistantMessage, error)
    Stream(ctx context.Context, req Request) (*Stream, error)
}
```

`Client` wraps any `Provider`:

```go
type Client struct { /* unexported */ }

func NewClient(p Provider) *Client
func (c *Client) Complete(ctx context.Context, req Request) (AssistantMessage, error)
func (c *Client) Stream(ctx context.Context, req Request) (*Stream, error)
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    gopiai "github.com/rahulSailesh-shah/go-pi-ai"
    "github.com/rahulSailesh-shah/go-pi-ai/openai"
)

func main() {
    provider, err := openai.NewProvider(openai.Config{
        APIKey: "your-api-key",
    })
    if err != nil {
        log.Fatal(err)
    }

    client := gopiai.NewClient(provider)

    msg, err := client.Complete(context.Background(), gopiai.Request{
        Model:        "gpt-4o",
        SystemPrompt: "You are a helpful assistant.",
        Messages: []gopiai.Message{
            gopiai.UserMessage{
                Contents: []gopiai.Content{
                    gopiai.TextContent{Text: "What is the capital of France?"},
                },
            },
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, c := range msg.Contents {
        if tc, ok := c.(gopiai.TextContent); ok {
            fmt.Println(tc.Text)
        }
    }
}
```

## Type Reference

### Content Types

`Content` is a sealed interface. There are three concrete implementations:

**TextContent** -- plain text:

```go
type TextContent struct {
    Text string `json:"text"`
}
// Type() returns "text"
```

**ImageContent** -- image as URL or base64. Set `URL` for image URLs. Set `Base64` + `MimeType` for inline base64 images:

```go
type ImageContent struct {
    URL      string `json:"url,omitempty"`
    Base64   string `json:"base64,omitempty"`
    MimeType string `json:"mime_type,omitempty"`
}
// Type() returns "image"
```

**ToolCall** -- a function/tool call from the assistant. `RawArguments` preserves the original JSON string for lossless replay in multi-turn conversations:

```go
type ToolCall struct {
    ID           string         `json:"id"`
    Name         string         `json:"name"`
    Arguments    map[string]any `json:"arguments"`
    RawArguments string         `json:"raw_arguments"`
}
// Type() returns "toolCall"
```

### Message Types

`Message` is a sealed interface with `Role() string` and `GetContents() []Content`. Three concrete types:

**UserMessage** (role: `"user"`):

```go
type UserMessage struct {
    Timestamp time.Time `json:"timestamp"`
    Contents  []Content `json:"-"`
}
```

**AssistantMessage** (role: `"assistant"`):

```go
type AssistantMessage struct {
    Contents   []Content  `json:"-"`
    Timestamp  time.Time  `json:"timestamp"`
    StopReason StopReason `json:"stop_reason"`
}
```

**ToolMessage** (role: `"tool"`):

```go
type ToolMessage struct {
    ToolCallID string    `json:"tool_call_id"`
    ToolName   string    `json:"tool_name"`
    Contents   []Content `json:"-"`
    IsError    bool      `json:"is_error"`
    Timestamp  time.Time `json:"timestamp"`
}
```

All three message types implement custom `MarshalJSON`/`UnmarshalJSON` for full round-trip JSON serialization, including their `[]Content` fields with type discriminators.

### Request

```go
type Request struct {
    Model        string    `json:"model"`
    SystemPrompt string    `json:"system_prompt,omitempty"`
    Messages     []Message `json:"-"`
    Tools        []Tool    `json:"tools,omitempty"`
    Temperature  *float64  `json:"temperature,omitempty"`
    MaxTokens    *int      `json:"max_tokens,omitempty"`
    Seed         *int      `json:"seed,omitempty"`
}
```

Optional scalar fields (`Temperature`, `MaxTokens`, `Seed`) use pointers so zero is distinguishable from unset. `Request` implements custom `MarshalJSON`/`UnmarshalJSON` for its `[]Message` field.

### Tool

```go
type Tool struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Parameters  map[string]any `json:"parameters"`
}
```

`Parameters` is a JSON Schema object describing the tool's input parameters.

### StopReason

```go
type StopReason string

const (
    StopReasonStop    StopReason = "stop"
    StopReasonLength  StopReason = "length"
    StopReasonToolUse StopReason = "tool_use"
    StopReasonAborted StopReason = "aborted"
    StopReasonError   StopReason = "error"
    StopReasonUnknown StopReason = "unknown"
)
```

### Errors

```go
var ErrInvalidConfig = errors.New("gopiai: invalid config")
```

Returned (wrapped) when provider config is invalid (e.g., missing API key).

## Streaming

`Stream` provides an iterator-based API. Call `Recv()` in a loop until `io.EOF` or error. Always `defer stream.Close()`.

```go
stream, err := client.Stream(ctx, req)
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for {
    event, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }

    switch e := event.(type) {
    case gopiai.EventStart:
        // streaming has begun
    case gopiai.EventTextStart:
        // text content block started (e.ContentIndex, e.Partial)
    case gopiai.EventTextDelta:
        // incremental text chunk (e.ContentIndex, e.Delta, e.Partial)
        fmt.Print(e.Delta)
    case gopiai.EventTextEnd:
        // text content block completed (e.ContentIndex, e.Content, e.Partial)
    case gopiai.EventToolcallStart:
        // tool call started (e.ContentIndex, e.Partial)
    case gopiai.EventToolcallDelta:
        // tool call arguments chunk (e.ContentIndex, e.Delta, e.Partial)
    case gopiai.EventToolcallEnd:
        // tool call completed (e.ContentIndex, e.ToolCall, e.Partial)
    case gopiai.EventDone:
        // streaming finished (e.Reason, e.Message)
        finalMessage = e.Message
    }
}
```

### Stream cancellation

`Stream` uses `context.Context` for cancellation. When the consumer calls `Close()`, the stream's internal context is cancelled, which:

1. Stops the producer goroutine from sending more events
2. Cancels the underlying HTTP request to the provider
3. Drains any remaining events in the channel

If the parent context passed to `Stream()` is cancelled (e.g., timeout), the same cleanup happens automatically.

### Event types

| Event | Fields | Description |
|-------|--------|-------------|
| `EventStart` | (none) | Streaming has begun |
| `EventTextStart` | `ContentIndex int`, `Partial AssistantMessage` | Text content block started |
| `EventTextDelta` | `ContentIndex int`, `Delta string`, `Partial AssistantMessage` | Incremental text chunk |
| `EventTextEnd` | `ContentIndex int`, `Content string`, `Partial AssistantMessage` | Text content block completed |
| `EventToolcallStart` | `ContentIndex int`, `Partial AssistantMessage` | Tool call started |
| `EventToolcallDelta` | `ContentIndex int`, `Delta string`, `Partial AssistantMessage` | Tool call arguments chunk |
| `EventToolcallEnd` | `ContentIndex int`, `ToolCall ToolCall`, `Partial AssistantMessage` | Tool call completed |
| `EventDone` | `Reason StopReason`, `Message AssistantMessage` | Streaming finished, `Message` is the complete response |
| `EventError` | `Error error` | Error during streaming (returned as error from `Recv()`) |

## Tool Calling (Multi-Turn)

A complete multi-turn tool calling flow:

```go
// 1. Build request with tools
req := gopiai.Request{
    Model:        "gpt-4o",
    SystemPrompt: "You are a helpful assistant.",
    Messages: []gopiai.Message{
        gopiai.UserMessage{
            Contents: []gopiai.Content{
                gopiai.TextContent{Text: "What's the weather in Tokyo?"},
            },
        },
    },
    Tools: []gopiai.Tool{
        {
            Name:        "getWeather",
            Description: "Get the weather for a given location",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "location": map[string]string{"type": "string"},
                },
                "required": []string{"location"},
            },
        },
    },
}

// 2. First call -- model returns tool calls
msg, err := client.Complete(ctx, req)
// msg.StopReason == gopiai.StopReasonToolUse

// 3. Append assistant message to conversation
req.Messages = append(req.Messages, msg)

// 4. Execute tool calls and append results
for _, content := range msg.Contents {
    if tc, ok := content.(gopiai.ToolCall); ok {
        result := executeMyTool(tc.Name, tc.Arguments)
        req.Messages = append(req.Messages, gopiai.ToolMessage{
            ToolCallID: tc.ID,
            ToolName:   tc.Name,
            Contents: []gopiai.Content{
                gopiai.TextContent{Text: result},
            },
        })
    }
}

// 5. Second call -- model generates final response with tool results
finalMsg, err := client.Complete(ctx, req)
// or use client.Stream(ctx, req) for streaming
```

## Using with Different Providers

Any OpenAI-compatible API works by setting `BaseURL` on the provider config:

```go
// OpenAI (default, no BaseURL needed)
provider, _ := openai.NewProvider(openai.Config{
    APIKey: os.Getenv("OPENAI_API_KEY"),
})

// NVIDIA
provider, _ := openai.NewProvider(openai.Config{
    APIKey:  os.Getenv("NVIDIA_API_KEY"),
    BaseURL: "https://integrate.api.nvidia.com/v1",
})

// Any OpenAI-compatible endpoint
provider, _ := openai.NewProvider(openai.Config{
    APIKey:  "key",
    BaseURL: "https://my-endpoint.com/v1",
})

// All use the same client
client := gopiai.NewClient(provider)
```

### OpenAI Provider Config

```go
type Config struct {
    APIKey     string       // Required. API key for authentication.
    BaseURL    string       // Optional. Defaults to OpenAI's API. Set for other providers.
    HTTPClient *http.Client // Optional. Custom HTTP client for proxies, timeouts, etc.
}
```

## Optional Request Parameters

```go
temp := 0.7
maxTokens := 1000
seed := 42

req := gopiai.Request{
    Model:       "gpt-4o",
    Temperature: &temp,
    MaxTokens:   &maxTokens,
    Seed:        &seed,
    // ...
}
```

## JSON Serialization

All types support full round-trip JSON serialization. This is useful for persisting conversations to disk.

```go
// Marshal a request (including all messages) to JSON
data, err := json.Marshal(req)

// Unmarshal back
var restored gopiai.Request
err = json.Unmarshal(data, &restored)
```

Content types are serialized with a `"type"` discriminator:

```json
{"type": "text", "data": {"text": "Hello"}}
{"type": "image", "data": {"url": "https://..."}}
{"type": "toolCall", "data": {"id": "call_123", "name": "getWeather", ...}}
```

Messages are serialized with a `"role"` discriminator:

```json
{"role": "user", "timestamp": "...", "contents": [...]}
{"role": "assistant", "timestamp": "...", "stop_reason": "stop", "contents": [...]}
{"role": "tool", "tool_call_id": "call_123", "tool_name": "getWeather", "contents": [...]}
```

Helper functions for working with message slices directly:

```go
// Marshal/unmarshal individual messages
raw, err := gopiai.MarshalMessage(msg)
msg, err := gopiai.UnmarshalMessage(raw)

// Marshal/unmarshal message slices
raws, err := gopiai.MarshalMessages(messages)
messages, err := gopiai.UnmarshalMessages(raws)
```

## Project Structure

```
.
├── client.go          # Provider interface, Client wrapper
├── types.go           # Content, Message, Tool, Request types + JSON serialization
├── events.go          # Streaming event types (EventStart, EventTextDelta, etc.)
├── stream.go          # Stream iterator (Recv/Close) with context cancellation
├── openai/
│   └── openai.go      # OpenAI-compatible provider implementation
└── cmd/example/
    └── main.go         # Example: Complete + Stream with tool calling
```

## Running the Example

1. Copy `.env.example` to `.env` and fill in your API key
2. Run:

```bash
go run cmd/example/main.go
```

## Dependencies

- [openai-go](https://github.com/openai/openai-go) -- OpenAI Go SDK (used by the `openai` provider)

## License

MIT

## Acknowledgments

Inspired by [pi-mono](https://github.com/badlogic/pi-mono) by [Mario Zechner](https://github.com/badlogic). Built with the [OpenAI Go SDK](https://github.com/openai/openai-go).
