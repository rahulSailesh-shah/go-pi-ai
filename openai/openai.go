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

			// Error from openai SDK if the chunk is not added to the accumulator
			if !acc.AddChunk(chunk) {
				sendFinalEvent(sctx, events, gopiai.EventDone{
					Reason:  gopiai.StopReasonError,
					Message: output,
					Err:     fmt.Errorf("failed to accumulate chunk"),
				})
				return
			}

			// If the finish reason is not empty, set the stop reason
			if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
				output.StopReason = stopReasonFromOpenAI(string(chunk.Choices[0].FinishReason))
			}

			// After model has finished streaming text, add the accumulated content to the output
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

			// If the tool call is finished, add the tool call to the output
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

			// Stream delta texts, these are not saved in the output, acc.JustFinishedContent() is used to get the text
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

			// Stream tool call deltas, these are not saved in the output, acc.JustFinishedToolCall() is used to get the tool call
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
			sendFinalEvent(sctx, events, gopiai.EventDone{
				Reason:  gopiai.StopReasonError,
				Message: output,
				Err:     fmt.Errorf("stream error: %w", err),
			})
			return
		}

		if sctx.Err() != nil {
			sendFinalEvent(sctx, events, gopiai.EventDone{
				Reason:  gopiai.StopReasonAborted,
				Message: output,
				Err:     sctx.Err(),
			})
			return
		}

	
		flushAccumulator(sctx, events, &acc, &output, &currentContentIndex)

		// If the model returned no usable content at all, surface as error
		// rather than silently completing with an empty assistant message.
		if len(output.Contents) == 0 {
			sendFinalEvent(sctx, events, gopiai.EventDone{
				Reason:  gopiai.StopReasonError,
				Message: output,
				Err:     fmt.Errorf("provider returned empty response (finish_reason=%s)", output.StopReason),
			})
			return
		}

		sendFinalEvent(sctx, events, gopiai.EventDone{
			Reason:  output.StopReason,
			Message: output,
		})
	}()

	return stream, nil
}

func flushAccumulator(
	ctx context.Context,
	events chan<- gopiai.Event,
	acc *openaiSDK.ChatCompletionAccumulator,
	output *gopiai.AssistantMessage,
	currentContentIndex *int,
) {
	if len(acc.Choices) == 0 {
		return
	}
	finalMsg := acc.Choices[0].Message

	seenToolCalls := make(map[string]bool)
	hasText := false
	for _, c := range output.Contents {
		switch v := c.(type) {
		case gopiai.ToolCall:
			seenToolCalls[v.ID] = true
		case gopiai.TextContent:
			hasText = true
		}
	}

	if !hasText && finalMsg.Content != "" {
		output.Contents = append(output.Contents, gopiai.TextContent{Text: finalMsg.Content})
	}

	for _, tc := range finalMsg.ToolCalls {
		if seenToolCalls[tc.ID] {
			continue
		}
		args := make(map[string]any)
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = make(map[string]any)
		}
		toolCall := gopiai.ToolCall{
			ID:           tc.ID,
			Name:         tc.Function.Name,
			Arguments:    args,
			RawArguments: tc.Function.Arguments,
		}
		output.Contents = append(output.Contents, toolCall)

		*currentContentIndex++
		idx := *currentContentIndex
		if !sendEvent(ctx, events, gopiai.EventToolcallStart{
			ContentIndex: idx,
			Partial:      *output,
		}) {
			return
		}
		if tc.Function.Arguments != "" {
			if !sendEvent(ctx, events, gopiai.EventToolcallDelta{
				ContentIndex: idx,
				Delta:        tc.Function.Arguments,
				Partial:      *output,
			}) {
				return
			}
		}
		if !sendEvent(ctx, events, gopiai.EventToolcallEnd{
			ContentIndex: idx,
			ToolCall:     toolCall,
			Partial:      *output,
		}) {
			return
		}
	}
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

// sendFinalEvent sends a terminal event, checking context cancellation.
// If context is cancelled, it returns false to avoid blocking when the buffer
// might be full and no one is reading.
func sendFinalEvent(ctx context.Context, events chan<- gopiai.Event, event gopiai.Event) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
