package gopiai

import (
	"encoding/json"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "tool_use"
	StopReasonAborted StopReason = "aborted"
	StopReasonError   StopReason = "error"
	StopReasonUnknown StopReason = "unknown"
)

type Request struct {
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	Messages     []Message `json:"-"`
	Tools        []Tool    `json:"tools,omitempty"`
	Temperature  *float64  `json:"temperature,omitempty"`
	MaxTokens    *int      `json:"max_tokens,omitempty"`
	Seed         *int      `json:"seed,omitempty"`
}

func (r Request) MarshalJSON() ([]byte, error) {
	msgs, err := MarshalMessages(r.Messages)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Model        string            `json:"model"`
		SystemPrompt string            `json:"system_prompt,omitempty"`
		Messages     []json.RawMessage `json:"messages"`
		Tools        []Tool            `json:"tools,omitempty"`
		Temperature  *float64          `json:"temperature,omitempty"`
		MaxTokens    *int              `json:"max_tokens,omitempty"`
		Seed         *int              `json:"seed,omitempty"`
	}{
		Model:        r.Model,
		SystemPrompt: r.SystemPrompt,
		Messages:     msgs,
		Tools:        r.Tools,
		Temperature:  r.Temperature,
		MaxTokens:    r.MaxTokens,
		Seed:         r.Seed,
	})
}

func (r *Request) UnmarshalJSON(data []byte) error {
	var raw struct {
		Model        string            `json:"model"`
		SystemPrompt string            `json:"system_prompt"`
		Messages     []json.RawMessage `json:"messages"`
		Tools        []Tool            `json:"tools"`
		Temperature  *float64          `json:"temperature"`
		MaxTokens    *int              `json:"max_tokens"`
		Seed         *int              `json:"seed"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	msgs, err := UnmarshalMessages(raw.Messages)
	if err != nil {
		return err
	}
	r.Model = raw.Model
	r.SystemPrompt = raw.SystemPrompt
	r.Messages = msgs
	r.Tools = raw.Tools
	r.Temperature = raw.Temperature
	r.MaxTokens = raw.MaxTokens
	r.Seed = raw.Seed
	return nil
}
