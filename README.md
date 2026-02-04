# go-pi-ai

A flexible, provider-agnostic Go library for interacting with AI language models. Currently supports OpenAI-compatible APIs (including NVIDIA's AI endpoints) with streaming and tool calling capabilities.

## Features

- 🔌 **Provider Abstraction**: Unified interface for multiple AI providers
- 🌊 **Streaming Support**: Real-time streaming of AI responses with event-based updates
- 🛠️ **Tool Calling**: Native support for function/tool calling with structured arguments
- 📦 **Type-Safe**: Strongly-typed message and content structures
- ⚙️ **Configuration Management**: Environment-based configuration with `.env` support
- 🔄 **Multi-Turn Conversations**: Support for complex conversation flows with tool interactions

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Usage](#usage)
  - [Configuration](#configuration)
  - [Basic Completion](#basic-completion)
  - [Streaming Responses](#streaming-responses)
  - [Tool Calling](#tool-calling)
- [API Reference](#api-reference)
- [Project Structure](#project-structure)
- [Contributing](#contributing)

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd go-pi-ai

# Install dependencies
go mod download

# Build the project
go build -o go-pi-ai
```

## Quick Start

1. **Create a `.env` file** with your API credentials:

```env
NVIDIA_API_URL="https://integrate.api.nvidia.com/v1"
NVIDIA_API_KEY="your-api-key-here"
```

2. **Run the example**:

```bash
go run main.go
```

## Architecture

The library is organized into several key packages:

```
pkg/
├── config/       # Configuration management and environment loading
├── models/       # Model registry and initialization
├── provider/     # Provider interface and implementations
│   └── openai/   # OpenAI-compatible provider implementation
└── types/        # Core type definitions and interfaces
```

### Core Concepts

- **Provider**: An abstraction over AI service providers (OpenAI, NVIDIA, etc.)
- **Model**: A specific AI model accessible through a provider
- **Context**: Represents a conversation with system prompt, messages, and available tools
- **Message**: User, assistant, or tool messages in a conversation
- **Content**: Text, image, or tool call content within messages
- **Tool**: Function definitions that the AI can invoke

## Usage

### Configuration

The library uses environment variables for configuration, loaded via `.env` files:

```go
import "ai/pkg/config"

// Get provider configuration
nvidiaConfig, ok := config.GetProvider(types.ApiNvidia)
if !ok {
    panic("provider not found")
}
```

**Supported Environment Variables:**

| Variable         | Description                   | Example                               |
| ---------------- | ----------------------------- | ------------------------------------- |
| `NVIDIA_API_URL` | NVIDIA API base URL           | `https://integrate.api.nvidia.com/v1` |
| `NVIDIA_API_KEY` | NVIDIA API authentication key | `nvapi-...`                           |

### Basic Completion

```go
import (
    "ai/pkg/config"
    "ai/pkg/models"
    "ai/pkg/types"
    "context"
)

// Get the model
nvidiaConfig, _ := config.GetProvider(types.ApiNvidia)
model := models.GetModel(types.ApiNvidia, nvidiaConfig.Models[0])

// Create a conversation
conversation := types.Context{
    SystemPrompt: "You are a helpful assistant.",
    Messages: []types.Message{
        types.UserMessage{
            Contents: []types.Content{
                types.TextContent{
                    Text: "What is the capital of France?",
                },
            },
        },
    },
}

// Get completion
message, err := model.Complete(context.Background(), conversation)
if err != nil {
    panic(err)
}

// Access the response
for _, content := range message.Contents {
    if content.Type() == "text" {
        textContent := content.(types.TextContent)
        println(textContent.Text)
    }
}
```

### Streaming Responses

```go
// Create a streaming request
stream := model.Stream(context.Background(), conversation)

// Process events in real-time
go func() {
    for event := range stream.Events {
        switch e := event.(type) {
        case types.EventTextDelta:
            // Handle text chunks as they arrive
            print(e.Delta)
        case types.EventDone:
            // Streaming completed
            println("\nDone!")
        }
    }
}()

// Get the final message
finalMessage := <-stream.Result
```

**Available Event Types:**

- `EventStart`: Streaming has begun
- `EventTextStart`: Text content block started
- `EventTextDelta`: Incremental text chunk received
- `EventTextEnd`: Text content block completed
- `EventToolcallStart`: Tool call started
- `EventToolcallDelta`: Tool call arguments chunk received
- `EventToolcallEnd`: Tool call completed
- `EventDone`: Streaming finished successfully
- `EventError`: An error occurred

### Tool Calling

Define tools that the AI can invoke:

```go
conversation := types.Context{
    SystemPrompt: "You are a helpful assistant.",
    Messages: []types.Message{
        types.UserMessage{
            Contents: []types.Content{
                types.TextContent{
                    Text: "What's the weather in Tokyo?",
                },
            },
        },
    },
    Tools: []types.Tool{
        {
            Name:        "getWeather",
            Description: "Get the weather for a given location",
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "location": map[string]string{
                        "type": "string",
                    },
                },
                "required": []string{"location"},
            },
        },
    },
}

// Get the response
message, _ := model.Complete(context.Background(), conversation)

// Extract tool calls
for _, content := range message.Contents {
    if content.Type() == "toolCall" {
        toolCall := content.(types.ToolCall)

        // Execute the tool with toolCall.Arguments
        result := executeWeatherTool(toolCall.Arguments["location"].(string))

        // Add tool result to conversation
        conversation.Messages = append(conversation.Messages, types.ToolMessage{
            ToolCallId: toolCall.ID,
            ToolName:   toolCall.Name,
            Contents: []types.Content{
                types.TextContent{
                    Text: result,
                },
            },
        })
    }
}

// Continue the conversation with tool results
finalMessage, _ := model.Complete(context.Background(), conversation)
```

## API Reference

### Core Types

#### `types.Context`

Represents a conversation context.

```go
type Context struct {
    SystemPrompt string    // System-level instructions
    Messages     []Message // Conversation history
    Tools        []Tool    // Available tools for the AI
}
```

#### `types.Message`

Interface for all message types (UserMessage, AssistantMessage, ToolMessage).

```go
type Message interface {
    Role() string        // "user", "assistant", or "toolResult"
    Content() []Content  // Message contents
}
```

#### `types.Content`

Interface for content types (TextContent, ImageContent, ToolCall).

```go
type Content interface {
    Type() string  // "text", "image", or "toolCall"
}
```

#### `types.Tool`

Defines a callable function/tool.

```go
type Tool struct {
    Name        string         // Tool identifier
    Description string         // What the tool does
    Parameters  map[string]any // JSON Schema for parameters
    Strict      bool           // Enforce strict parameter validation
}
```

### Provider Interface

```go
type Provider interface {
    // Stream returns a streaming response
    Stream(ctx context.Context, conversation types.Context) types.AssistantMessageEventStream

    // Complete returns a single complete response
    Complete(ctx context.Context, conversation types.Context) (types.AssistantMessage, error)
}
```

### Model Registry

```go
// Add a model to the registry
models.AddModel(providerType types.ModelProvider, id string, model provider.Provider)

// Get a model from the registry
models.GetModel(providerType types.ModelProvider, id string) provider.Provider
```

## Project Structure

```
.
├── main.go                          # Example usage
├── go.mod                           # Go module definition
├── go.sum                           # Dependency checksums
├── .env                             # Environment configuration (not in git)
├── .gitignore                       # Git ignore rules
└── pkg/
    ├── config/
    │   └── config.go                # Configuration loading and management
    ├── models/
    │   └── models.go                # Model registry and initialization
    ├── provider/
    │   ├── provider.go              # Provider interface definition
    │   └── openai/
    │       ├── openai.go            # OpenAI-compatible provider implementation
    │       └── streaming.go         # Streaming event handling
    └── types/
        └── types.go                 # Core type definitions
```

## Adding New Providers

To add support for a new AI provider:

1. **Implement the `Provider` interface** in `pkg/provider/<provider-name>/`:

```go
type MyProvider struct {
    // provider-specific fields
}

func (p *MyProvider) Stream(ctx context.Context, conversation types.Context) types.AssistantMessageEventStream {
    // Implementation
}

func (p *MyProvider) Complete(ctx context.Context, conversation types.Context) (types.AssistantMessage, error) {
    // Implementation
}
```

2. **Add configuration** in `pkg/config/config.go`:

```go
const (
    ApiMyProvider ModelProvider = "myprovider"
)

// In init():
AppConfig.Providers[types.ApiMyProvider] = ProviderConfig{
    BaseURL: getEnv("MYPROVIDER_API_URL"),
    APIKey:  getEnv("MYPROVIDER_API_KEY"),
    Models:  []string{"model-id"},
}
```

3. **Register models** in `pkg/models/models.go`:

```go
if config, ok := config.GetProvider(types.ApiMyProvider); ok {
    for _, modelID := range config.Models {
        provider := myprovider.NewMyProvider(config, modelID)
        AddModel(types.ApiMyProvider, modelID, provider)
    }
}
```

## Dependencies

- [openai-go](https://github.com/openai/openai-go) - OpenAI Go SDK
- [godotenv](https://github.com/joho/godotenv) - Environment variable loading
- [gjson](https://github.com/tidwall/gjson) - JSON parsing utilities
- [sjson](https://github.com/tidwall/sjson) - JSON modification utilities

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

[Add your license here]

## Acknowledgments

- Built with the [OpenAI Go SDK](https://github.com/openai/openai-go)
- Supports OpenAI-compatible APIs
