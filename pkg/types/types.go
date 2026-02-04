package types

import (
	"time"
)

// --------------- Content ---------------
type Content interface {
	Type() string
	isContent()
}

type TextContent struct {
	Text string
}

func (t TextContent) Type() string {
	return "text"
}

func (t TextContent) isContent() {}

type ImageContent struct {
	Data     string
	MimeType string
}

func (i ImageContent) Type() string {
	return "image"
}

func (i ImageContent) isContent() {}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

func (t ToolCall) Type() string {
	return "toolCall"
}

func (t ToolCall) isContent() {}

// --------------- Messages ---------------
type Message interface {
	isMessage()
	Role() string
	Content() []Content
}

type UserMessage struct {
	Timestamp time.Time
	Contents  []Content
}

func (u UserMessage) isMessage() {}

func (u UserMessage) Role() string {
	return "user"
}

func (u UserMessage) Content() []Content {
	return u.Contents
}

type AssistantMessage struct {
	Contents     []Content
	Timestamp    time.Time
	Provider     ModelProvider
	ErrorMessage *string
	StopReason   StopReason
}

func (a AssistantMessage) Role() string {
	return "assistant"
}

func (a AssistantMessage) isMessage() {}

func (a AssistantMessage) Content() []Content {
	return a.Contents
}

type ToolMessage struct {
	ToolCallId string
	ToolName   string
	Contents   []Content
	Details    *any
	IsError    bool
	Timestamp  time.Time
}

func (t ToolMessage) Role() string {
	return "toolResult"
}

func (t ToolMessage) isMessage() {}

func (t ToolMessage) Content() []Content {
	return t.Contents
}

// --------------- Tool ---------------
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Strict      bool
}

// --------------- Context ---------------
type Context struct {
	SystemPrompt string
	Messages     []Message
	Tools        []Tool
}

type ModelProvider string

const (
	ApiNvidia ModelProvider = "nvidia"
)

// --------------- AssistantMessageEventStream ---------------
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "toolUse"
	StopReasonAborted StopReason = "aborted"
	StopReasonError   StopReason = "error"
)

type AssistantMessageEvent interface {
	isMessageEvent()
}

type EventStart struct {
	Partial AssistantMessage
}

func (e EventStart) isMessageEvent() {}

type EventTextStart struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (e EventTextStart) isMessageEvent() {}

type EventTextDelta struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (e EventTextDelta) isMessageEvent() {}

type EventTextEnd struct {
	ContentIndex int
	Content      string
	Partial      AssistantMessage
}

func (e EventTextEnd) isMessageEvent() {}

type EventToolcallStart struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (e EventToolcallStart) isMessageEvent() {}

type EventToolcallDelta struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (e EventToolcallDelta) isMessageEvent() {}

type EventToolcallEnd struct {
	ContentIndex int
	ToolCall     ToolCall
	Partial      AssistantMessage
}

func (e EventToolcallEnd) isMessageEvent() {}

type EventDone struct {
	Reason  StopReason
	Message AssistantMessage
}

func (e EventDone) isMessageEvent() {}

type EventError struct {
	Reason StopReason
	Error  AssistantMessage
}

func (e EventError) isMessageEvent() {}

type AssistantMessageEventStream struct {
	Events chan AssistantMessageEvent
	Result chan AssistantMessage
	Err    chan error
}
