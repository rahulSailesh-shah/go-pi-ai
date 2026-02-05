package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	openaiSDK "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/rahulSailesh-shah/go-pi-ai/types"
)

type Config struct {
	URL    string
	APIKey string
}

type Provider struct {
	config       Config
	modelID      string
	providerType types.ModelProvider
	client       *openaiSDK.Client
	mu           sync.Mutex
}

func New(config Config, modelID string, providerType types.ModelProvider) *Provider {
	return &Provider{
		config:       config,
		modelID:      modelID,
		providerType: providerType,
	}
}

func (p *Provider) Model() string {
	return p.modelID
}

func (p *Provider) ProviderType() types.ModelProvider {
	return p.providerType
}

func (p *Provider) getClient() (*openaiSDK.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		return p.client, nil
	}

	// Validate config
	if p.config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	opts := []option.RequestOption{}
	opts = append(opts, option.WithAPIKey(p.config.APIKey))

	if p.config.URL != "" {
		opts = append(opts, option.WithBaseURL(p.config.URL))
	}

	client := openaiSDK.NewClient(opts...)
	p.client = &client
	return p.client, nil
}

func (p *Provider) Stream(ctx context.Context, conversation types.Context) types.AssistantMessageEventStream {
	stream := types.NewAssistantMessageEventStream()

	go func() {
		output := types.AssistantMessage{
			Contents:  []types.Content{},
			Provider:  p.providerType,
			Timestamp: time.Now(),
		}

		params := buildParams(p.modelID, conversation)

		// Get or create client lazily
		client, err := p.getClient()
		if err != nil {
			stream.Err <- fmt.Errorf("failed to create client: %w", err)
			return
		}

		openaiStream := client.Chat.Completions.NewStreaming(ctx, params)

		acc := openaiSDK.ChatCompletionAccumulator{}

		// Start event
		stream.Events <- types.EventStart{}
		currentContentIndex := -1
		currentBlockType := ""

		for openaiStream.Next() {
			chunk := openaiStream.Current()

			if !acc.AddChunk(chunk) {
				// Handle error
				stream.Err <- fmt.Errorf("failed to add chunk")
				return
			}

			// Update stop reason
			if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
				output.StopReason = stopReasonFromOpenAI(string(chunk.Choices[0].FinishReason))
			}

			// Handle finished content block
			if content, ok := acc.JustFinishedContent(); ok && content != "" {
				stream.Events <- types.EventTextEnd{
					ContentIndex: currentContentIndex,
					Content:      content,
					Partial:      output,
				}
				currentBlockType = ""
				output.Contents = append(output.Contents, types.TextContent{
					Text: content,
				})
			}

			// Handle finished tool call block
			if toolCall, ok := acc.JustFinishedToolCall(); ok {
				args := make(map[string]any)
				if err := json.Unmarshal([]byte(toolCall.Arguments), &args); err != nil {
					args = make(map[string]interface{})
				}
				tc := types.ToolCall{
					ID:        toolCall.ID,
					Name:      toolCall.Name,
					Arguments: args,
				}
				stream.Events <- types.EventToolcallEnd{
					ContentIndex: currentContentIndex,
					ToolCall:     tc,
					Partial:      output,
				}
				currentBlockType = ""
				output.Contents = append(output.Contents, tc)
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			// Handle text delta
			if delta.Content != "" {
				fmt.Println(".")
				if currentBlockType != "text" {
					currentContentIndex++
					currentBlockType = "text"
					stream.Events <- types.EventTextStart{
						ContentIndex: currentContentIndex,
						Partial:      output,
					}
				}
				stream.Events <- types.EventTextDelta{
					ContentIndex: currentContentIndex,
					Delta:        delta.Content,
					Partial:      output,
				}
			}

			// Handle tool call delta
			if len(delta.ToolCalls) > 0 {
				for _, toolCallDelta := range delta.ToolCalls {
					if currentBlockType != "toolCall" {
						currentContentIndex++
						currentBlockType = "toolCall"
						stream.Events <- types.EventToolcallStart{
							ContentIndex: currentContentIndex,
							Partial:      output,
						}
					}
					if toolCallDelta.Function.Arguments != "" {
						stream.Events <- types.EventToolcallDelta{
							ContentIndex: currentContentIndex,
							Delta:        toolCallDelta.Function.Arguments,
							Partial:      output,
						}
					}
				}
			}
		}

		if err := openaiStream.Err(); err != nil {
			stream.Err <- err
			return
		}

		if ctx.Err() != nil {
			stream.Err <- ctx.Err()
			return
		}

		stream.Events <- types.EventDone{
			Reason:  output.StopReason,
			Message: output,
		}

		stream.Result <- output
		stream.Close()
	}()

	return stream
}

func (p *Provider) Complete(ctx context.Context, conversation types.Context) (types.AssistantMessage, error) {
	params := buildParams(p.modelID, conversation)

	client, err := p.getClient()
	if err != nil {
		return types.AssistantMessage{}, fmt.Errorf("failed to create client: %w", err)
	}

	response, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return types.AssistantMessage{}, fmt.Errorf("completion failed: %w", err)
	}

	output := types.AssistantMessage{
		Provider:  p.providerType,
		Timestamp: time.Now(),
		Contents:  []types.Content{},
	}

	if len(response.Choices) > 0 {
		output.StopReason = stopReasonFromOpenAI(string(response.Choices[0].FinishReason))
		msg := response.Choices[0].Message

		if msg.Content != "" {
			output.Contents = append(output.Contents, types.TextContent{
				Text: msg.Content,
			})
		}

		for _, tc := range msg.ToolCalls {
			args := make(map[string]any)
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = make(map[string]any)
			}
			output.Contents = append(output.Contents, types.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
			})
		}
	}

	return output, nil
}

func buildParams(modelID string, conversation types.Context) openaiSDK.ChatCompletionNewParams {
	messages := buildMessages(conversation)
	tools := buildTools(conversation.Tools)

	return openaiSDK.ChatCompletionNewParams{
		Messages: messages,
		Model:    modelID,
		Tools:    tools,
		Seed:     openaiSDK.Int(0),
	}
}

func buildMessages(conversation types.Context) []openaiSDK.ChatCompletionMessageParamUnion {
	openaiMessages := []openaiSDK.ChatCompletionMessageParamUnion{}

	if conversation.SystemPrompt != "" {
		openaiMessages = append(openaiMessages, openaiSDK.SystemMessage(conversation.SystemPrompt))
	}

	for _, message := range conversation.Messages {
		switch msg := message.(type) {
		case types.UserMessage:
			openaiMessages = append(openaiMessages, openaiSDK.UserMessage(buildUserContent(msg.Contents)))

		case types.AssistantMessage:
			openaiMessages = append(openaiMessages, buildAssistantMessage(msg))

		case types.ToolMessage:
			openaiMessages = append(openaiMessages, openaiSDK.ToolMessage(buildToolContent(msg.Contents), msg.ToolCallId))
		}
	}

	return openaiMessages
}

func buildAssistantMessage(msg types.AssistantMessage) openaiSDK.ChatCompletionMessageParamUnion {
	toolCalls := []openaiSDK.ChatCompletionMessageToolCallUnionParam{}
	textParts := []openaiSDK.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{}

	for _, content := range msg.Contents {
		switch c := content.(type) {
		case types.TextContent:
			textParts = append(textParts, openaiSDK.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
				OfText: &openaiSDK.ChatCompletionContentPartTextParam{Text: c.Text},
			})

		case types.ToolCall:
			toolCalls = append(toolCalls, openaiSDK.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openaiSDK.ChatCompletionMessageFunctionToolCallParam{
					ID: c.ID,
					Function: openaiSDK.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name: c.Name,
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

func buildUserContent(contents []types.Content) []openaiSDK.ChatCompletionContentPartUnionParam {
	parts := []openaiSDK.ChatCompletionContentPartUnionParam{}

	for _, content := range contents {
		switch c := content.(type) {
		case types.TextContent:
			parts = append(parts, openaiSDK.ChatCompletionContentPartUnionParam{
				OfText: &openaiSDK.ChatCompletionContentPartTextParam{Text: c.Text},
			})

		case types.ImageContent:
			parts = append(parts, openaiSDK.ChatCompletionContentPartUnionParam{
				OfImageURL: &openaiSDK.ChatCompletionContentPartImageParam{
					ImageURL: openaiSDK.ChatCompletionContentPartImageImageURLParam{URL: c.Data},
				},
			})
		}
	}

	return parts
}

func buildToolContent(contents []types.Content) []openaiSDK.ChatCompletionContentPartTextParam {
	parts := []openaiSDK.ChatCompletionContentPartTextParam{}

	for _, content := range contents {
		if c, ok := content.(types.TextContent); ok {
			parts = append(parts, openaiSDK.ChatCompletionContentPartTextParam{Text: c.Text})
		}
	}

	return parts
}

func buildTools(tools []types.Tool) []openaiSDK.ChatCompletionToolUnionParam {
	openaiTools := []openaiSDK.ChatCompletionToolUnionParam{}

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

func stopReasonFromOpenAI(reason string) types.StopReason {
	switch reason {
	case "stop":
		return types.StopReasonStop
	case "length":
		return types.StopReasonLength
	case "tool_calls":
		return types.StopReasonToolUse
	case "content_filter":
		return types.StopReasonAborted
	default:
		return types.StopReasonUnknown
	}
}
