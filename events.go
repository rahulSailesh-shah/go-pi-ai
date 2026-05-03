package gopiai

// Event represents a streaming event from the assistant.
type Event interface {
	isEvent()
}

type EventStart struct{}

func (e EventStart) isEvent() {}

type EventTextStart struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (e EventTextStart) isEvent() {}

type EventTextDelta struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (e EventTextDelta) isEvent() {}

type EventTextEnd struct {
	ContentIndex int
	Content      string
	Partial      AssistantMessage
}

func (e EventTextEnd) isEvent() {}

type EventToolcallStart struct {
	ContentIndex int
	Partial      AssistantMessage
}

func (e EventToolcallStart) isEvent() {}

type EventToolcallDelta struct {
	ContentIndex int
	Delta        string
	Partial      AssistantMessage
}

func (e EventToolcallDelta) isEvent() {}

type EventToolcallEnd struct {
	ContentIndex int
	ToolCall     ToolCall
	Partial      AssistantMessage
}

func (e EventToolcallEnd) isEvent() {}

// EventDone is the terminal event. Err is non-nil when the stream ended due to
// an error. Message contains whatever partial content was received before the error.
type EventDone struct {
	Reason  StopReason
	Message AssistantMessage
	Err     error
}

func (e EventDone) isEvent() {}
