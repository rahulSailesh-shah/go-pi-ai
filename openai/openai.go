package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	openaiSDK "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	gopiai "github.com/rahulSailesh-shah/go-pi-ai"
)

type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type Provider struct {
	client *openaiSDK.Client
}

// NewProvider creates a new OpenAI-compatible provider.
func NewProvider(cfg Config) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w: API key is required", gopiai.ErrInvalidConfig)
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}

	client := openaiSDK.NewClient(opts...)
	return &Provider{client: &client}, nil
}

// providerError wraps an API error and signals whether it is retryable.
type providerError struct {
	err       error
	retryable bool
}

func (e *providerError) Error() string   { return e.err.Error() }
func (e *providerError) Unwrap() error   { return e.err }
func (e *providerError) Retryable() bool { return e.retryable }

func wrapProviderError(err error, msg string) error {
	wrapped := fmt.Errorf("%s: %w", msg, err)
	retryable := false
	var apiErr *openaiSDK.Error
	if errors.As(err, &apiErr) {
		retryable = apiErr.StatusCode == 429 || apiErr.StatusCode == 503
	}
	return &providerError{err: wrapped, retryable: retryable}
}

// Complete sends a non-streaming chat completion request.
func (c *Provider) Complete(ctx context.Context, req gopiai.Request) (gopiai.AssistantMessage, error) {
	params := buildParams(req)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return gopiai.AssistantMessage{}, wrapProviderError(err, "completion failed")
	}

	output := gopiai.AssistantMessage{
		Timestamp: time.Now(),
		Contents:  []gopiai.Content{},
	}

	if len(response.Choices) > 0 {
		output.StopReason = stopReasonFromOpenAI(string(response.Choices[0].FinishReason))
		msg := response.Choices[0].Message

		if msg.Content != "" {
			output.Contents = append(output.Contents, gopiai.TextContent{Text: msg.Content})
		}

		for _, tc := range msg.ToolCalls {
			args := make(map[string]any)
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = make(map[string]any)
			}
			output.Contents = append(output.Contents, gopiai.ToolCall{
				ID:           tc.ID,
				Name:         tc.Function.Name,
				Arguments:    args,
				RawArguments: tc.Function.Arguments,
			})
		}
	}

	return output, nil
}

// Stream starts a streaming chat completion request and returns a Stream
// that the caller reads with Recv() in a loop.
func (c *Provider) Stream(ctx context.Context, req gopiai.Request) (*gopiai.Stream, error) {
	params := buildParams(req)

	stream, events := gopiai.NewStream(ctx)

	go func() {
		defer close(events)

		openaiStream := c.client.Chat.Completions.NewStreaming(stream.Context(), params)

		output := gopiai.AssistantMessage{
			Timestamp: time.Now(),
			Contents:  []gopiai.Content{},
			Usage:     gopiai.Usage{},
		}

		acc := openaiSDK.ChatCompletionAccumulator{}

		sctx := stream.Context()

		if !sendEvent(sctx, events, gopiai.EventStart{}) {
			return
		}

		currentContentIndex := -1
		currentBlockType := ""

		for openaiStream.Next() {
			chunk := openaiStream.Current()
			if u := usageFromOpenAI(chunk); u.TotalTokens > 0 {
				output.Usage = u
			}

			if !acc.AddChunk(chunk) {
				sendFinalEvent(events, gopiai.EventDone{
					Reason:  gopiai.StopReasonError,
					Message: output,
					Err:     fmt.Errorf("failed to accumulate chunk"),
				})
				return
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
				output.StopReason = stopReasonFromOpenAI(string(chunk.Choices[0].FinishReason))
			}

			if content, ok := acc.JustFinishedContent(); ok && content != "" {
				output.Contents = append(output.Contents, gopiai.TextContent{Text: content})
				if !sendEvent(sctx, events, gopiai.EventTextEnd{
					ContentIndex: currentContentIndex,
					Content:      content,
					Partial:      output,
				}) {
					return
				}
				currentBlockType = ""
			}

			if toolCall, ok := acc.JustFinishedToolCall(); ok {
				args := make(map[string]any)
				if err := json.Unmarshal([]byte(toolCall.Arguments), &args); err != nil {
					args = make(map[string]any)
				}
				tc := gopiai.ToolCall{
					ID:           toolCall.ID,
					Name:         toolCall.Name,
					Arguments:    args,
					RawArguments: toolCall.Arguments,
				}
				output.Contents = append(output.Contents, tc)
				if !sendEvent(sctx, events, gopiai.EventToolcallEnd{
					ContentIndex: currentContentIndex,
					ToolCall:     tc,
					Partial:      output,
				}) {
					return
				}
				currentBlockType = ""
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				if currentBlockType != "text" {
					currentContentIndex += 1
					currentBlockType = "text"
					if !sendEvent(sctx, events, gopiai.EventTextStart{
						ContentIndex: currentContentIndex,
						Partial:      output,
					}) {
						return
					}
				}
				if !sendEvent(sctx, events, gopiai.EventTextDelta{
					ContentIndex: currentContentIndex,
					Delta:        delta.Content,
					Partial:      output,
				}) {
					return
				}
			}

			if len(delta.ToolCalls) > 0 {
				for _, toolCallDelta := range delta.ToolCalls {
					if currentBlockType != "toolCall" {
						currentContentIndex++
						currentBlockType = "toolCall"
						if !sendEvent(sctx, events, gopiai.EventToolcallStart{
							ContentIndex: currentContentIndex,
							Partial:      output,
						}) {
							return
						}
					}
					if toolCallDelta.Function.Arguments != "" {
						if !sendEvent(sctx, events, gopiai.EventToolcallDelta{
							ContentIndex: currentContentIndex,
							Delta:        toolCallDelta.Function.Arguments,
							Partial:      output,
						}) {
							return
						}
					}
				}
			}
		}

		if err := openaiStream.Err(); err != nil {
			sendFinalEvent(events, gopiai.EventDone{
				Reason:  gopiai.StopReasonError,
				Message: output,
				Err:     fmt.Errorf("stream error: %w", err),
			})
			return
		}

		if sctx.Err() != nil {
			sendFinalEvent(events, gopiai.EventDone{
				Reason:  gopiai.StopReasonAborted,
				Message: output,
				Err:     sctx.Err(),
			})
			return
		}

		sendFinalEvent(events, gopiai.EventDone{
			Reason:  output.StopReason,
			Message: output,
		})
	}()

	return stream, nil
}

// sendEvent sends an event to the channel, returning false if the context
// is cancelled (consumer called Close() or parent context was cancelled).
func sendEvent(ctx context.Context, events chan<- gopiai.Event, event gopiai.Event) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// sendFinalEvent sends a terminal event unconditionally, ignoring context
// cancellation. This ensures the consumer always receives the final EventDone
// even when the context triggered the termination.
func sendFinalEvent(events chan<- gopiai.Event, event gopiai.Event) {
	events <- event
}

