package openai

import (
	"ai/pkg/types"
	"context"
	"encoding/json"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type OpenAIConfig struct {
	URL    string
	APIKey string
}

type OpenAIProvider struct {
	client        *openai.Client
	ModelID       string
	ModelProvider types.ModelProvider
}

func NewOpenAIProvider(config OpenAIConfig, modelID string, modelProvider types.ModelProvider) *OpenAIProvider {
	opts := []option.RequestOption{}
	opts = append(opts, option.WithAPIKey(config.APIKey))

	if config.URL != "" {
		opts = append(opts, option.WithBaseURL(config.URL))
	}

	client := openai.NewClient(opts...)
	return &OpenAIProvider{
		client:        &client,
		ModelID:       modelID,
		ModelProvider: modelProvider,
	}
}

func (p *OpenAIProvider) Stream(ctx context.Context, conversation types.Context) types.AssistantMessageEventStream {
	stream := types.AssistantMessageEventStream{
		Events: make(chan types.AssistantMessageEvent),
		Result: make(chan types.AssistantMessage),
		Err:    make(chan error),
	}

	go func() {
		defer close(stream.Events)
		defer close(stream.Result)
		defer close(stream.Err)

		output := types.AssistantMessage{
			Contents:  []types.Content{},
			Provider:  p.ModelProvider,
			Timestamp: time.Now(),
		}

		params := buildParams(p.ModelID, conversation)
		openaiStream := p.client.Chat.Completions.NewStreaming(ctx, params)

		acc := openai.ChatCompletionAccumulator{}

		// Start event
		stream.Events <- types.EventStart{Partial: output}
		currentContentIndex := -1
		currentBlockType := ""

		for openaiStream.Next() {
			chunk := openaiStream.Current()

			if !acc.AddChunk(chunk) {
				// Handle error
				return
			}

			// Update stop reason
			if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
				output.StopReason = types.StopReason(chunk.Choices[0].FinishReason)
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
				output.Contents = append(output.Contents, types.ToolCall{
					ID:        toolCall.ID,
					Name:      toolCall.Name,
					Arguments: args,
				})
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			// Handle text delta
			if delta.Content != "" {
				if currentBlockType != "text" {
					currentContentIndex++
					currentBlockType = "text"
					// Start new text block
					stream.Events <- types.EventTextStart{
						ContentIndex: currentContentIndex,
						Partial:      output,
					}
				}
				// Emit delta event
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
					// Emit delta event for arguments
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

		// Check for stream errors
		if err := openaiStream.Err(); err != nil {
			// TODO: Handle error
			stream.Err <- err
			return
		}
		// Check for context cancellation
		if ctx.Err() != nil {
			// TODO: Handle error
			stream.Err <- ctx.Err()
			return
		}

		// Send done event
		stream.Events <- types.EventDone{
			Reason:  output.StopReason,
			Message: output,
		}

		stream.Result <- output
	}()
	return stream
}

func (p *OpenAIProvider) Complete(ctx context.Context, conversation types.Context) (types.AssistantMessage, error) {
	params := buildParams(p.ModelID, conversation)
	response, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return types.AssistantMessage{}, err
	}
	output := types.AssistantMessage{
		Provider:  p.ModelProvider,
		Timestamp: time.Now(),
		Contents:  []types.Content{},
	}
	if len(response.Choices) > 0 {
		output.StopReason = types.StopReason(response.Choices[0].FinishReason)

		// Handle content
		msg := response.Choices[0].Message
		if msg.Content != "" {
			output.Contents = append(output.Contents, types.TextContent{
				Text: msg.Content,
			})
		}

		// Handle tool calls
		for _, tc := range msg.ToolCalls {
			// args := make(map[string]any)
			// if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			// 	args = make(map[string]any)
			// }
			output.Contents = append(output.Contents, types.ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				// Arguments: args,
			})
		}
	}
	return output, nil
}

func buildParams(modelID string, conversation types.Context) openai.ChatCompletionNewParams {
	messages := buildMessages(conversation.Messages)
	tools := buildTools(conversation.Tools)
	return openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    modelID,
		Tools:    tools,
		Seed:     openai.Int(0),
	}
}

func buildMessages(messages []types.Message) []openai.ChatCompletionMessageParamUnion {
	openaiMessages := []openai.ChatCompletionMessageParamUnion{}
	for _, message := range messages {
		switch message.Role() {
		case "user":
			if len(message.Content()) == 0 {
				continue
			}
			msg := openai.ChatCompletionUserMessageParam{}
			msg.Content = openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{},
			}
			for _, content := range message.Content() {
				switch content.Type() {
				case "text":
					msg.Content.OfArrayOfContentParts = append(msg.Content.OfArrayOfContentParts, openai.ChatCompletionContentPartUnionParam{
						OfText: &openai.ChatCompletionContentPartTextParam{Text: content.(types.TextContent).Text},
					})
				case "image":
					msg.Content.OfArrayOfContentParts = append(msg.Content.OfArrayOfContentParts, openai.ChatCompletionContentPartUnionParam{
						// TODO: Handle image content convert to base64
						OfImageURL: &openai.ChatCompletionContentPartImageParam{ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: content.(types.ImageContent).Data}},
					})
				}
			}
			openaiMessages = append(openaiMessages, openai.ChatCompletionMessageParamUnion{OfUser: &msg})

		case "assistant":
			if len(message.Content()) == 0 {
				continue
			}
			msg := openai.ChatCompletionAssistantMessageParam{}
			msg.ToolCalls = []openai.ChatCompletionMessageToolCallUnionParam{}
			msg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
				OfArrayOfContentParts: []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{},
			}
			for _, content := range message.Content() {
				switch content.Type() {
				case "text":
					msg.Content.OfArrayOfContentParts = append(msg.Content.OfArrayOfContentParts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
						OfText: &openai.ChatCompletionContentPartTextParam{Text: content.(types.TextContent).Text},
					})
				case "toolCall":
					msg.ToolCalls = append(msg.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: content.(types.ToolCall).ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name: content.(types.ToolCall).Name,
							},
						},
					})
				default:
					continue
				}
			}
			openaiMessages = append(openaiMessages, openai.ChatCompletionMessageParamUnion{OfAssistant: &msg})

		case "toolResult":
			if len(message.Content()) == 0 {
				continue
			}
			toolMessages := []openai.ChatCompletionContentPartTextParam{}
			for _, content := range message.Content() {
				if content.Type() == "text" {
					toolMessages = append(toolMessages, openai.ChatCompletionContentPartTextParam{Text: content.(types.TextContent).Text})
				}
			}
			openaiMessages = append(openaiMessages, openai.ToolMessage(toolMessages, message.(types.ToolMessage).ToolCallId))
		default:
			continue
		}
	}
	return openaiMessages
}

func buildTools(tools []types.Tool) []openai.ChatCompletionToolUnionParam {
	openaiTools := []openai.ChatCompletionToolUnionParam{}
	for _, tool := range tools {
		toolDef := openai.ChatCompletionFunctionTool(
			openai.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(tool.Description),
				Parameters:  tool.Parameters,
				Strict:      openai.Bool(tool.Strict),
			},
		)
		openaiTools = append(openaiTools, toolDef)
	}
	return openaiTools
}
