package gopiai

import (
	"context"
	"errors"
	"net"
	"time"
)

var (
	ErrInvalidConfig = errors.New("gopiai: invalid config")
)

// Provider is the interface that provider implementations must satisfy.
type Provider interface {
	Complete(ctx context.Context, req Request) (AssistantMessage, error)
	Stream(ctx context.Context, req Request) (*Stream, error)
}

// Logger is the interface for observability hooks.
type Logger interface {
	Log(ctx context.Context, msg string, fields map[string]any)
}

// RetryableError is implemented by provider errors that signal retryability.
type RetryableError interface {
	Retryable() bool
}

// ClientOption configures a Client.
type ClientOption func(*clientConfig)

type clientConfig struct {
	maxAttempts int
	baseDelay   time.Duration
	logger      Logger
	timeout     time.Duration
}

// WithRetry sets the maximum number of attempts and base backoff delay for
// Complete calls. maxAttempts=1 means no retry (the default).
func WithRetry(maxAttempts int, baseDelay time.Duration) ClientOption {
	return func(c *clientConfig) {
		c.maxAttempts = maxAttempts
		c.baseDelay = baseDelay
	}
}

// WithLogger attaches a logger for observability.
func WithLogger(l Logger) ClientOption {
	return func(c *clientConfig) {
		c.logger = l
	}
}

// WithTimeout sets a per-Complete deadline applied on top of the caller's context.
// Does not apply to Stream — pass a context with deadline directly to Stream instead.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) {
		c.timeout = d
	}
}

// Client wraps a Provider and is the main entry point for using the SDK.
type Client struct {
	provider Provider
	cfg      clientConfig
}

// NewClient creates a new Client that delegates to the given Provider.
func NewClient(p Provider, opts ...ClientOption) *Client {
	cfg := clientConfig{maxAttempts: 1}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Client{provider: p, cfg: cfg}
}

// Complete sends a non-streaming chat completion request, retrying on
// retryable errors up to the configured maxAttempts.
func (c *Client) Complete(ctx context.Context, req Request) (AssistantMessage, error) {
	if c.cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.timeout)
		defer cancel()
	}

	maxAttempts := c.cfg.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	if c.cfg.logger != nil {
		c.cfg.logger.Log(ctx, "complete", map[string]any{"model": req.Model})
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := backoffDelay(c.cfg.baseDelay, attempt)
			if c.cfg.logger != nil {
				c.cfg.logger.Log(ctx, "retrying", map[string]any{"attempt": attempt + 1, "delay": delay.String()})
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return AssistantMessage{}, ctx.Err()
			}
		}

		msg, err := c.provider.Complete(ctx, req)
		if err == nil {
			return msg, nil
		}

		if !isRetryable(err) {
			return AssistantMessage{}, err
		}
		lastErr = err
	}

	return AssistantMessage{}, lastErr
}

// Stream starts a streaming chat completion request.
// Streaming retries and per-request timeouts are not applied here — manage
// stream lifetime via the context you pass in. Use Complete for retry semantics.
func (c *Client) Stream(ctx context.Context, req Request) (*Stream, error) {
	if c.cfg.logger != nil {
		c.cfg.logger.Log(ctx, "stream", map[string]any{"model": req.Model})
	}
	return c.provider.Stream(ctx, req)
}

func isRetryable(err error) bool {
	var re RetryableError
	if errors.As(err, &re) {
		return re.Retryable()
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

func backoffDelay(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	// exponential: base * 2^(attempt-1)
	delay := base * (1 << uint(attempt-1))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}
