package types

import (
	"errors"
	"time"
)

const (
	// Model provider constants
	ProviderNvidia    ModelProvider = "nvidia"
	ProviderOpenAI    ModelProvider = "openai"
	ProviderAnthropic ModelProvider = "anthropic"
	ProviderMistral   ModelProvider = "mistral"
	ProviderCustom    ModelProvider = "custom"
)

var (
	// ErrProviderNotFound is returned when a provider is not found
	ErrProviderNotFound = errors.New("provider not found")
	// ErrModelNotFound is returned when a model is not found
	ErrModelNotFound = errors.New("model not found")
	// ErrConfigInvalid is returned when configuration is invalid
	ErrConfigInvalid = errors.New("invalid configuration")
)

// Content represents any content that can be part of a message
type Content interface {
	Type() string
	isContent()
}

// TextContent represents plain text content
type TextContent struct {
	Text string
}

func (t TextContent) Type() string {
	return "text"
}

func (t TextContent) isContent() {}

// ImageContent represents image content
type ImageContent struct {
	Data     string
	MimeType string
}

func (i ImageContent) Type() string {
	return "image"
}

func (i ImageContent) isContent() {}

// ToolCall represents a function/tool call
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

func (t ToolCall) Type() string {
	return "toolCall"
}

func (t ToolCall) isContent() {}

// Message represents any message in a conversation
type Message interface {
	isMessage()
	Role() string
	Content() []Content
}

// UserMessage represents a message from the user
type UserMessage struct {
	Timestamp time.Time
	Contents  []Content
}

func (m UserMessage) isMessage() {}

func (m UserMessage) Role() string {
	return "user"
}

func (m UserMessage) Content() []Content {
	return m.Contents
}

// AssistantMessage represents a message from the assistant
type AssistantMessage struct {
	Contents     []Content
	Timestamp    time.Time
	Provider     ModelProvider
	ErrorMessage *string
	StopReason   StopReason
}

func (m AssistantMessage) isMessage() {}

func (m AssistantMessage) Role() string {
	return "assistant"
}

func (m AssistantMessage) Content() []Content {
	return m.Contents
}

// ToolMessage represents a response from a tool call
type ToolMessage struct {
	ToolCallId string
	ToolName   string
	Contents   []Content
	Details    *any
	IsError    bool
	Timestamp  time.Time
}

func (m ToolMessage) isMessage() {}

func (m ToolMessage) Role() string {
	return "tool"
}

func (m ToolMessage) Content() []Content {
	return m.Contents
}

// Tool represents a function/tool available to the assistant
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Context represents the full conversation context
type Context struct {
	SystemPrompt string
	Messages     []Message
	Tools        []Tool
	Metadata     map[string]any
}

// ModelProvider identifies a model provider
type ModelProvider string

// StopReason indicates why generation stopped
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "tool_use"
	StopReasonAborted StopReason = "aborted"
	StopReasonError   StopReason = "error"
	StopReasonUnknown StopReason = "unknown"
)

// AssistantMessageEvent represents a streaming event
type AssistantMessageEvent interface {
	isMessageEvent()
}

// EventStart represents the start of a response
type EventStart struct{}

func (e EventStart) isMessageEvent() {}

// EventTextStart represents the start of text content
type EventTextStart struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (e EventTextStart) isMessageEvent() {}

// EventTextDelta represents incremental text updates
type EventTextDelta struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (e EventTextDelta) isMessageEvent() {}

// EventTextEnd represents the end of text content
type EventTextEnd struct {
	ContentIndex int
	Content      string
	Partial      AssistantMessage
}

func (e EventTextEnd) isMessageEvent() {}

// EventToolcallStart represents the start of a tool call
type EventToolcallStart struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (e EventToolcallStart) isMessageEvent() {}

// EventToolcallDelta represents incremental tool call updates
type EventToolcallDelta struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (e EventToolcallDelta) isMessageEvent() {}

// EventToolcallEnd represents the end of a tool call
type EventToolcallEnd struct {
	ContentIndex int
	ToolCall     ToolCall
	Partial      AssistantMessage
}

func (e EventToolcallEnd) isMessageEvent() {}

// EventDone represents the completion of a response
type EventDone struct {
	Reason  StopReason
	Message AssistantMessage
}

func (e EventDone) isMessageEvent() {}

// EventError represents an error during streaming
type EventError struct {
	Reason StopReason
	Error  AssistantMessage
}

func (e EventError) isMessageEvent() {}

// AssistantMessageEventStream manages streaming events
type AssistantMessageEventStream struct {
	Events chan AssistantMessageEvent
	Result chan AssistantMessage
	Err    chan error
}

// NewAssistantMessageEventStream creates a new event stream
func NewAssistantMessageEventStream() AssistantMessageEventStream {
	return AssistantMessageEventStream{
		Events: make(chan AssistantMessageEvent),
		Result: make(chan AssistantMessage),
		Err:    make(chan error),
	}
}

// Close closes all channels in the stream
func (s AssistantMessageEventStream) Close() {
	close(s.Events)
	close(s.Result)
	close(s.Err)
}
