package openai

import (
	"context"
	"encoding/json"
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

// Complete sends a non-streaming chat completion request.
func (c *Provider) Complete(ctx context.Context, req gopiai.Request) (gopiai.AssistantMessage, error) {
	params := buildParams(req)

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return gopiai.AssistantMessage{}, fmt.Errorf("completion failed: %w", err)
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
				sendEvent(sctx, events, gopiai.EventError{Error: fmt.Errorf("failed to accumulate chunk")})
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
					currentContentIndex++
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
			sendEvent(sctx, events, gopiai.EventError{Error: err})
			return
		}

		if sctx.Err() != nil {
			sendEvent(sctx, events, gopiai.EventError{Error: sctx.Err()})
			return
		}

		sendEvent(sctx, events, gopiai.EventDone{
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

func buildParams(req gopiai.Request) openaiSDK.ChatCompletionNewParams {
	messages := buildMessages(req)
	tools := buildTools(req.Tools)

	params := openaiSDK.ChatCompletionNewParams{
		Messages: messages,
		Model:    req.Model,
		Tools:    tools,
	}

	if req.Temperature != nil {
		params.Temperature = openaiSDK.Float(*req.Temperature)
	}
	if req.MaxTokens != nil {
		params.MaxTokens = openaiSDK.Int(int64(*req.MaxTokens))
	}
	if req.Seed != nil {
		params.Seed = openaiSDK.Int(int64(*req.Seed))
	}

	return params
}

func buildMessages(req gopiai.Request) []openaiSDK.ChatCompletionMessageParamUnion {
	var messages []openaiSDK.ChatCompletionMessageParamUnion

	if req.SystemPrompt != "" {
		messages = append(messages, openaiSDK.SystemMessage(req.SystemPrompt))
	}

	for _, message := range req.Messages {
		switch msg := message.(type) {
		case gopiai.UserMessage:
			messages = append(messages, openaiSDK.UserMessage(buildUserContent(msg.Contents)))
		case gopiai.AssistantMessage:
			messages = append(messages, buildAssistantMessage(msg))
		case gopiai.ToolMessage:
			messages = append(messages, openaiSDK.ToolMessage(buildToolContent(msg.Contents), msg.ToolCallID))
		}
	}

	return messages
}

func buildAssistantMessage(msg gopiai.AssistantMessage) openaiSDK.ChatCompletionMessageParamUnion {
	var toolCalls []openaiSDK.ChatCompletionMessageToolCallUnionParam
	var textParts []openaiSDK.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion

	for _, content := range msg.Contents {
		switch c := content.(type) {
		case gopiai.TextContent:
			textParts = append(textParts, openaiSDK.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
				OfText: &openaiSDK.ChatCompletionContentPartTextParam{Text: c.Text},
			})
		case gopiai.ToolCall:
			toolCalls = append(toolCalls, openaiSDK.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openaiSDK.ChatCompletionMessageFunctionToolCallParam{
					ID: c.ID,
					Function: openaiSDK.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      c.Name,
						Arguments: c.RawArguments,
					},
				},
			})
		}
	}

	assistantMsg := openaiSDK.ChatCompletionAssistantMessageParam{}
	if len(textParts) > 0 {
		assistantMsg.Content = openaiSDK.ChatCompletionAssistantMessageParamContentUnion{
			OfArrayOfContentParts: textParts,
		}
	}
	if len(toolCalls) > 0 {
		assistantMsg.ToolCalls = toolCalls
	}

	return openaiSDK.ChatCompletionMessageParamUnion{
		OfAssistant: &assistantMsg,
	}
}

func buildUserContent(contents []gopiai.Content) []openaiSDK.ChatCompletionContentPartUnionParam {
	var parts []openaiSDK.ChatCompletionContentPartUnionParam

	for _, content := range contents {
		switch c := content.(type) {
		case gopiai.TextContent:
			parts = append(parts, openaiSDK.ChatCompletionContentPartUnionParam{
				OfText: &openaiSDK.ChatCompletionContentPartTextParam{Text: c.Text},
			})
		case gopiai.ImageContent:
			url := c.URL
			if url == "" && c.Base64 != "" && c.MimeType != "" {
				url = fmt.Sprintf("data:%s;base64,%s", c.MimeType, c.Base64)
			}
			if url != "" {
				parts = append(parts, openaiSDK.ChatCompletionContentPartUnionParam{
					OfImageURL: &openaiSDK.ChatCompletionContentPartImageParam{
						ImageURL: openaiSDK.ChatCompletionContentPartImageImageURLParam{URL: url},
					},
				})
			}
		}
	}

	return parts
}

func buildToolContent(contents []gopiai.Content) []openaiSDK.ChatCompletionContentPartTextParam {
	var parts []openaiSDK.ChatCompletionContentPartTextParam

	for _, content := range contents {
		if c, ok := content.(gopiai.TextContent); ok {
			parts = append(parts, openaiSDK.ChatCompletionContentPartTextParam{Text: c.Text})
		}
	}

	return parts
}

func buildTools(tools []gopiai.Tool) []openaiSDK.ChatCompletionToolUnionParam {
	var openaiTools []openaiSDK.ChatCompletionToolUnionParam

	for _, tool := range tools {
		toolDef := openaiSDK.ChatCompletionFunctionTool(
			openaiSDK.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openaiSDK.String(tool.Description),
				Parameters:  tool.Parameters,
				Strict:      openaiSDK.Bool(false),
			},
		)
		openaiTools = append(openaiTools, toolDef)
	}

	return openaiTools
}

func stopReasonFromOpenAI(reason string) gopiai.StopReason {
	switch reason {
	case "stop":
		return gopiai.StopReasonStop
	case "length":
		return gopiai.StopReasonLength
	case "tool_calls":
		return gopiai.StopReasonToolUse
	case "content_filter":
		return gopiai.StopReasonAborted
	default:
		return gopiai.StopReasonUnknown
	}
}

func usageFromOpenAI(chunk openaiSDK.ChatCompletionChunk) gopiai.Usage {
	return gopiai.Usage{
		PromptTokens:     int(chunk.Usage.PromptTokens),
		CompletionTokens: int(chunk.Usage.CompletionTokens),
		TotalTokens:      int(chunk.Usage.TotalTokens),
	}
}
