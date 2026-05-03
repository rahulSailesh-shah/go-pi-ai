package gopiai

import (
	"encoding/json"
	"fmt"
	"time"
)

type Message interface {
	isMessage()
	Role() string
	GetContents() []Content
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

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

type AssistantMessage struct {
	Contents   []Content  `json:"-"`
	Timestamp  time.Time  `json:"timestamp"`
	StopReason StopReason `json:"stop_reason"`
	Usage      Usage      `json:"usage"`
}

func (m AssistantMessage) isMessage()             {}
func (m AssistantMessage) Role() string           { return "assistant" }
func (m AssistantMessage) GetContents() []Content { return m.Contents }

func (m AssistantMessage) MarshalJSON() ([]byte, error) {
	contents, err := marshalContents(m.Contents)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Role       string            `json:"role"`
		Contents   []json.RawMessage `json:"contents"`
		Timestamp  time.Time         `json:"timestamp"`
		StopReason StopReason        `json:"stop_reason"`
		Usage      Usage             `json:"usage"`
	}{Role: "assistant", Contents: contents, Timestamp: m.Timestamp, StopReason: m.StopReason, Usage: m.Usage})
}

func (m *AssistantMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Contents   []json.RawMessage `json:"contents"`
		Timestamp  time.Time         `json:"timestamp"`
		StopReason StopReason        `json:"stop_reason"`
		Usage      Usage             `json:"usage"`
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
	return nil
}

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

func MarshalMessage(m Message) (json.RawMessage, error) {
	return json.Marshal(m)
}

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
