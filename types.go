package gopiai

import (
	"encoding/json"
	"fmt"
	"time"
)

// Content represents any content that can be part of a message.
type Content interface {
	Type() string
	isContent()
}

// TextContent represents plain text content.
type TextContent struct {
	Text string `json:"text"`
}

func (t TextContent) Type() string { return "text" }
func (t TextContent) isContent()   {}

// ImageContent represents image content, either as a URL or base64-encoded data.
// Set URL for image URLs. Set Base64 and MimeType for inline base64 images.
type ImageContent struct {
	URL      string `json:"url,omitempty"`
	Base64   string `json:"base64,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

func (i ImageContent) Type() string { return "image" }
func (i ImageContent) isContent()   {}

// ToolCall represents a function/tool call from the assistant.
// RawArguments preserves the original JSON so tool-call replay is lossless.
type ToolCall struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Arguments    map[string]any `json:"arguments"`
	RawArguments string         `json:"raw_arguments"`
}

func (t ToolCall) Type() string { return "toolCall" }
func (t ToolCall) isContent()   {}

// marshalContent wraps a Content value with a "type" discriminator for JSON.
func marshalContent(c Content) (json.RawMessage, error) {
	type contentWrapper struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return json.Marshal(contentWrapper{Type: c.Type(), Data: data})
}

// unmarshalContent reads a typed JSON wrapper and returns the concrete Content.
func unmarshalContent(raw json.RawMessage) (Content, error) {
	var wrapper struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	switch wrapper.Type {
	case "text":
		var c TextContent
		return c, json.Unmarshal(wrapper.Data, &c)
	case "image":
		var c ImageContent
		return c, json.Unmarshal(wrapper.Data, &c)
	case "toolCall":
		var c ToolCall
		return c, json.Unmarshal(wrapper.Data, &c)
	default:
		return nil, fmt.Errorf("unknown content type: %s", wrapper.Type)
	}
}

func marshalContents(contents []Content) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(contents))
	for i, c := range contents {
		raw, err := marshalContent(c)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func unmarshalContents(raws []json.RawMessage) ([]Content, error) {
	result := make([]Content, len(raws))
	for i, raw := range raws {
		c, err := unmarshalContent(raw)
		if err != nil {
			return nil, err
		}
		result[i] = c
	}
	return result, nil
}

// Message represents any message in a conversation.
type Message interface {
	isMessage()
	Role() string
	GetContents() []Content
}

// UserMessage represents a message from the user.
type UserMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Contents  []Content `json:"-"`
}

func (m UserMessage) isMessage()             {}
func (m UserMessage) Role() string           { return "user" }
func (m UserMessage) GetContents() []Content { return m.Contents }

func (m UserMessage) MarshalJSON() ([]byte, error) {
	contents, err := marshalContents(m.Contents)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Role      string            `json:"role"`
		Timestamp time.Time         `json:"timestamp"`
		Contents  []json.RawMessage `json:"contents"`
	}{Role: "user", Timestamp: m.Timestamp, Contents: contents})
}

func (m *UserMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Timestamp time.Time         `json:"timestamp"`
		Contents  []json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	contents, err := unmarshalContents(raw.Contents)
	if err != nil {
		return err
	}
	m.Timestamp = raw.Timestamp
	m.Contents = contents
	return nil
}

// AssistantMessage represents a message from the assistant.

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
type AssistantMessage struct {
	Contents     []Content  `json:"-"`
	Timestamp    time.Time  `json:"timestamp"`
	StopReason   StopReason `json:"stop_reason"`
	Usage        Usage      `json:"usage"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

func (m AssistantMessage) isMessage()             {}
func (m AssistantMessage) Role() string           { return "assistant" }
func (m AssistantMessage) GetContents() []Content { return m.Contents }

func (m AssistantMessage) HasError() bool {
	return m.ErrorMessage != ""
}

func (m AssistantMessage) HasPartialContent() bool {
	return len(m.Contents) > 0
}

func (m AssistantMessage) MarshalJSON() ([]byte, error) {
	contents, err := marshalContents(m.Contents)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Role         string            `json:"role"`
		Contents     []json.RawMessage `json:"contents"`
		Timestamp    time.Time         `json:"timestamp"`
		StopReason   StopReason        `json:"stop_reason"`
		Usage        Usage             `json:"usage"`
		ErrorMessage string            `json:"error_message,omitempty"`
	}{Role: "assistant", Contents: contents, Timestamp: m.Timestamp, StopReason: m.StopReason, Usage: m.Usage, ErrorMessage: m.ErrorMessage})
}

func (m *AssistantMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Contents     []json.RawMessage `json:"contents"`
		Timestamp    time.Time         `json:"timestamp"`
		StopReason   StopReason        `json:"stop_reason"`
		Usage        Usage             `json:"usage"`
		ErrorMessage string            `json:"error_message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	contents, err := unmarshalContents(raw.Contents)
	if err != nil {
		return err
	}
	m.Contents = contents
	m.Timestamp = raw.Timestamp
	m.StopReason = raw.StopReason
	m.Usage = raw.Usage
	m.ErrorMessage = raw.ErrorMessage
	return nil
}

// ToolMessage represents a response from a tool call.
type ToolMessage struct {
	ToolCallID string    `json:"tool_call_id"`
	ToolName   string    `json:"tool_name"`
	Contents   []Content `json:"-"`
	IsError    bool      `json:"is_error"`
	Timestamp  time.Time `json:"timestamp"`
}

func (m ToolMessage) isMessage()             {}
func (m ToolMessage) Role() string           { return "tool" }
func (m ToolMessage) GetContents() []Content { return m.Contents }

func (m ToolMessage) MarshalJSON() ([]byte, error) {
	contents, err := marshalContents(m.Contents)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Role       string            `json:"role"`
		ToolCallID string            `json:"tool_call_id"`
		ToolName   string            `json:"tool_name"`
		Contents   []json.RawMessage `json:"contents"`
		IsError    bool              `json:"is_error"`
		Timestamp  time.Time         `json:"timestamp"`
	}{Role: "tool", ToolCallID: m.ToolCallID, ToolName: m.ToolName, Contents: contents, IsError: m.IsError, Timestamp: m.Timestamp})
}

func (m *ToolMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		ToolCallID string            `json:"tool_call_id"`
		ToolName   string            `json:"tool_name"`
		Contents   []json.RawMessage `json:"contents"`
		IsError    bool              `json:"is_error"`
		Timestamp  time.Time         `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	contents, err := unmarshalContents(raw.Contents)
	if err != nil {
		return err
	}
	m.ToolCallID = raw.ToolCallID
	m.ToolName = raw.ToolName
	m.Contents = contents
	m.IsError = raw.IsError
	m.Timestamp = raw.Timestamp
	return nil
}

// MarshalMessage marshals any Message to JSON with a "role" discriminator.
func MarshalMessage(m Message) (json.RawMessage, error) {
	return json.Marshal(m)
}

// UnmarshalMessage reads a JSON message and returns the concrete Message type.
func UnmarshalMessage(data json.RawMessage) (Message, error) {
	var peek struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return nil, err
	}
	switch peek.Role {
	case "user":
		var m UserMessage
		return m, json.Unmarshal(data, &m)
	case "assistant":
		var m AssistantMessage
		return m, json.Unmarshal(data, &m)
	case "tool":
		var m ToolMessage
		return m, json.Unmarshal(data, &m)
	default:
		return nil, fmt.Errorf("unknown message role: %s", peek.Role)
	}
}

// MarshalMessages marshals a slice of Messages to JSON.
func MarshalMessages(messages []Message) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(messages))
	for i, m := range messages {
		raw, err := MarshalMessage(m)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

// UnmarshalMessages reads a JSON array of messages into concrete Message types.
func UnmarshalMessages(raws []json.RawMessage) ([]Message, error) {
	result := make([]Message, len(raws))
	for i, raw := range raws {
		m, err := UnmarshalMessage(raw)
		if err != nil {
			return nil, err
		}
		result[i] = m
	}
	return result, nil
}

// Tool represents a function/tool available to the assistant.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Request is the input to Complete and Stream.
type Request struct {
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	Messages     []Message `json:"-"`
	Tools        []Tool    `json:"tools,omitempty"`
	Temperature  *float64  `json:"temperature,omitempty"`
	MaxTokens    *int      `json:"max_tokens,omitempty"`
	Seed         *int      `json:"seed,omitempty"`
}

// StopReason indicates why generation stopped.
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "tool_use"
	StopReasonAborted StopReason = "aborted"
	StopReasonError   StopReason = "error"
	StopReasonUnknown StopReason = "unknown"
)
