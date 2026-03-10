package gopiai

import (
	"context"
	"errors"
)

var (
	ErrInvalidConfig = errors.New("gopiai: invalid config")
)

// Provider is the interface that provider implementations must satisfy.
type Provider interface {
	Complete(ctx context.Context, req Request) (AssistantMessage, error)
	Stream(ctx context.Context, req Request) (*Stream, error)
}

// Client wraps a Provider and is the main entry point for using the SDK.
type Client struct {
	provider Provider
}

// NewClient creates a new Client that delegates to the given Provider.
func NewClient(p Provider) *Client {
	return &Client{provider: p}
}

// Complete sends a non-streaming chat completion request.
func (c *Client) Complete(ctx context.Context, req Request) (AssistantMessage, error) {
	return c.provider.Complete(ctx, req)
}

// Stream starts a streaming chat completion request.
func (c *Client) Stream(ctx context.Context, req Request) (*Stream, error) {
	return c.provider.Stream(ctx, req)
}
