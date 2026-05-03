package openai

import (
	"fmt"

	openaiSDK "github.com/openai/openai-go/v3"
	gopiai "github.com/rahulSailesh-shah/go-pi-ai"
)

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
