package gopiai

import (
	"context"
	"io"
	"sync"
)

// Stream provides an iterator-based API for consuming streaming events.
// Call Recv() in a loop until it returns io.EOF (stream complete) or an error.
// Always call Close() when done (use defer).
type Stream struct {
	events chan Event
	cancel context.CancelFunc
	ctx    context.Context
	once   sync.Once
}

// NewStream creates a new Stream. The provided context is used for cancellation —
// if the parent context is cancelled, the producer will stop.
// Close() also cancels the stream's context.
func NewStream(ctx context.Context) (*Stream, chan<- Event) {
	ctx, cancel := context.WithCancel(ctx)
	ch := make(chan Event, 1)
	s := &Stream{
		events: ch,
		ctx:    ctx,
		cancel: cancel,
	}
	return s, ch
}

// Recv returns the next event from the stream.
// Returns io.EOF when the stream is complete.
func (s *Stream) Recv() (Event, error) {
	event, ok := <-s.events
	if !ok {
		return nil, io.EOF
	}
	return event, nil
}

// Close signals that the consumer is done reading and releases resources.
func (s *Stream) Close() error {
	s.once.Do(func() {
		s.cancel()
		for range s.events {
		}
	})
	return nil
}

// Context returns the stream's context. Producers should select on
// ctx.Done() to detect cancellation from Close() or the parent context.
func (s *Stream) Context() context.Context {
	return s.ctx
}
