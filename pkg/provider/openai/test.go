package openai

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"time"
// 	"your-module/types" // Replace with your actual module path
// )

// // ContentBlock represents a block being built during streaming
// type ContentBlock interface {
// 	GetType() string
// }

// // TextBlock represents a text content block being built
// type TextBlock struct {
// 	Type string
// 	Text string
// }

// func (t *TextBlock) GetType() string { return "text" }

// // ThinkingBlock represents a thinking/reasoning block being built
// type ThinkingBlock struct {
// 	Type              string
// 	Thinking          string
// 	ThinkingSignature string
// }

// func (t *ThinkingBlock) GetType() string { return "thinking" }

// // ToolCallBlock represents a tool call block being built
// type ToolCallBlock struct {
// 	Type        string
// 	ID          string
// 	Name        string
// 	Arguments   map[string]interface{}
// 	PartialArgs string // Accumulates JSON fragments
// }

// func (t *ToolCallBlock) GetType() string { return "toolCall" }

// // Stream implements the OpenAI completions streaming logic
// func (p *OpenAIProvider) Stream(
// 	ctx context.Context,
// 	model types.Model,
// 	conversation types.Context,
// 	options *types.StreamOptions,
// ) types.AssistantMessageEventStream {
// 	stream := types.AssistantMessageEventStream{
// 		Events: make(chan types.AssistantMessageEvent),
// 		Result: make(chan types.AssistantMessage),
// 		Err:    make(chan error),
// 	}
// 	go func() {
// 		defer close(stream.Events)
// 		defer close(stream.Result)
// 		defer close(stream.Err)
// 		// Initialize the output message
// 		output := types.AssistantMessage{
// 			Role:     "assistant",
// 			Content:  []types.Content{},
// 			API:      model.API,
// 			Provider: model.Provider,
// 			Model:    model.ID,
// 			Usage: types.Usage{
// 				Input:       0,
// 				Output:      0,
// 				CacheRead:   0,
// 				CacheWrite:  0,
// 				TotalTokens: 0,
// 				Cost: types.Cost{
// 					Input:      0,
// 					Output:     0,
// 					CacheRead:  0,
// 					CacheWrite: 0,
// 					Total:      0,
// 				},
// 			},
// 			StopReason: types.StopReasonStop,
// 			Timestamp:  time.Now().UnixMilli(),
// 		}
// 		// Create OpenAI client and build request params
// 		client, err := p.createClient(model, conversation, options)
// 		if err != nil {
// 			p.handleError(&stream, &output, err, options)
// 			return
// 		}
// 		params, err := p.buildParams(model, conversation, options)
// 		if err != nil {
// 			p.handleError(&stream, &output, err, options)
// 			return
// 		}
// 		// Call onPayload callback if provided
// 		if options != nil && options.OnPayload != nil {
// 			options.OnPayload(params)
// 		}
// 		// Create the streaming request
// 		openaiStream, err := client.CreateChatCompletionStream(ctx, params)
// 		if err != nil {
// 			p.handleError(&stream, &output, err, options)
// 			return
// 		}
// 		defer openaiStream.Close()
// 		// Send start event
// 		stream.Events <- types.EventStart{Partial: output}
// 		// State management for current block
// 		var currentBlock ContentBlock
// 		blockIndex := func() int { return len(output.Content) - 1 }
// 		// finishCurrentBlock emits the *_end event for the current block
// 		finishCurrentBlock := func(block ContentBlock) {
// 			if block == nil {
// 				return
// 			}
// 			switch b := block.(type) {
// 			case *TextBlock:
// 				stream.Events <- types.EventTextEnd{
// 					ContentIndex: blockIndex(),
// 					Content:      b.Text,
// 					Partial:      output,
// 				}
// 			case *ThinkingBlock:
// 				stream.Events <- types.EventThinkingEnd{
// 					ContentIndex: blockIndex(),
// 					Content:      b.Thinking,
// 					Partial:      output,
// 				}
// 			case *ToolCallBlock:
// 				// Parse the accumulated JSON arguments
// 				args := make(map[string]interface{})
// 				if b.PartialArgs != "" {
// 					if err := json.Unmarshal([]byte(b.PartialArgs), &args); err != nil {
// 						// If parsing fails, use empty object
// 						args = make(map[string]interface{})
// 					}
// 				}
// 				b.Arguments = args
// 				stream.Events <- types.EventToolcallEnd{
// 					ContentIndex: blockIndex(),
// 					ToolCall: types.ToolCall{
// 						Type:      "toolCall",
// 						ID:        b.ID,
// 						Name:      b.Name,
// 						Arguments: b.Arguments,
// 					},
// 					Partial: output,
// 				}
// 			}
// 		}
// 		// Process the stream
// 		for {
// 			select {
// 			case <-ctx.Done():
// 				finishCurrentBlock(currentBlock)
// 				p.handleError(&stream, &output, ctx.Err(), options)
// 				return
// 			default:
// 			}
// 			chunk, err := openaiStream.Recv()
// 			if err == io.EOF {
// 				break
// 			}
// 			if err != nil {
// 				finishCurrentBlock(currentBlock)
// 				p.handleError(&stream, &output, err, options)
// 				return
// 			}
// 			// Update usage information if present
// 			if chunk.Usage != nil {
// 				cachedTokens := 0
// 				if chunk.Usage.PromptTokensDetails != nil {
// 					cachedTokens = chunk.Usage.PromptTokensDetails.CachedTokens
// 				}
// 				reasoningTokens := 0
// 				if chunk.Usage.CompletionTokensDetails != nil {
// 					reasoningTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
// 				}
// 				input := chunk.Usage.PromptTokens - cachedTokens
// 				outputTokens := chunk.Usage.CompletionTokens + reasoningTokens
// 				output.Usage = types.Usage{
// 					Input:       input,
// 					Output:      outputTokens,
// 					CacheRead:   cachedTokens,
// 					CacheWrite:  0,
// 					TotalTokens: input + outputTokens + cachedTokens,
// 					Cost: types.Cost{
// 						Input:      0,
// 						Output:     0,
// 						CacheRead:  0,
// 						CacheWrite: 0,
// 						Total:      0,
// 					},
// 				}
// 				// Calculate cost (you'll need to implement this)
// 				p.calculateCost(model, &output.Usage)
// 			}
// 			// Process choices
// 			if len(chunk.Choices) == 0 {
// 				continue
// 			}
// 			choice := chunk.Choices[0]
// 			// Update stop reason if present
// 			if choice.FinishReason != "" {
// 				output.StopReason = p.mapStopReason(choice.FinishReason)
// 			}
// 			// Process delta content
// 			if choice.Delta == nil {
// 				continue
// 			}
// 			delta := choice.Delta
// 			// Handle text content
// 			if delta.Content != "" {
// 				// Check if we need to start a new text block
// 				if currentBlock == nil || currentBlock.GetType() != "text" {
// 					finishCurrentBlock(currentBlock)
// 					currentBlock = &TextBlock{
// 						Type: "text",
// 						Text: "",
// 					}
// 					output.Content = append(output.Content, types.TextContent{
// 						Type: "text",
// 						Text: "",
// 					})
// 					stream.Events <- types.EventTextStart{
// 						ContentIndex: blockIndex(),
// 						Partial:      output,
// 					}
// 				}
// 				// Append to current text block
// 				if textBlock, ok := currentBlock.(*TextBlock); ok {
// 					textBlock.Text += delta.Content
// 					// Update the content in output
// 					if idx := blockIndex(); idx >= 0 && idx < len(output.Content) {
// 						if tc, ok := output.Content[idx].(types.TextContent); ok {
// 							tc.Text = textBlock.Text
// 							output.Content[idx] = tc
// 						}
// 					}
// 					stream.Events <- types.EventTextDelta{
// 						ContentIndex: blockIndex(),
// 						Delta:        delta.Content,
// 						Partial:      output,
// 					}
// 				}
// 			}
// 			// Handle reasoning/thinking content
// 			// Check multiple field names for compatibility
// 			reasoningFields := []string{"reasoning_content", "reasoning", "reasoning_text"}
// 			var foundReasoningField string
// 			var reasoningContent string
// 			for _, field := range reasoningFields {
// 				if content := p.getReasoningField(delta, field); content != "" {
// 					foundReasoningField = field
// 					reasoningContent = content
// 					break
// 				}
// 			}
// 			if foundReasoningField != "" {
// 				// Check if we need to start a new thinking block
// 				if currentBlock == nil || currentBlock.GetType() != "thinking" {
// 					finishCurrentBlock(currentBlock)
// 					currentBlock = &ThinkingBlock{
// 						Type:              "thinking",
// 						Thinking:          "",
// 						ThinkingSignature: foundReasoningField,
// 					}
// 					output.Content = append(output.Content, types.ThinkingContent{
// 						Type:              "thinking",
// 						Thinking:          "",
// 						ThinkingSignature: foundReasoningField,
// 					})
// 					stream.Events <- types.EventThinkingStart{
// 						ContentIndex: blockIndex(),
// 						Partial:      output,
// 					}
// 				}
// 				// Append to current thinking block
// 				if thinkingBlock, ok := currentBlock.(*ThinkingBlock); ok {
// 					thinkingBlock.Thinking += reasoningContent
// 					// Update the content in output
// 					if idx := blockIndex(); idx >= 0 && idx < len(output.Content) {
// 						if tc, ok := output.Content[idx].(types.ThinkingContent); ok {
// 							tc.Thinking = thinkingBlock.Thinking
// 							output.Content[idx] = tc
// 						}
// 					}
// 					stream.Events <- types.EventThinkingDelta{
// 						ContentIndex: blockIndex(),
// 						Delta:        reasoningContent,
// 						Partial:      output,
// 					}
// 				}
// 			}
// 			// Handle tool calls
// 			if len(delta.ToolCalls) > 0 {
// 				for _, toolCall := range delta.ToolCalls {
// 					// Check if we need to start a new tool call block
// 					// Start new block if: no current block, wrong type, or different ID
// 					needNewBlock := currentBlock == nil ||
// 						currentBlock.GetType() != "toolCall" ||
// 						(toolCall.ID != "" && currentBlock.(*ToolCallBlock).ID != toolCall.ID)
// 					if needNewBlock {
// 						finishCurrentBlock(currentBlock)
// 						currentBlock = &ToolCallBlock{
// 							Type:        "toolCall",
// 							ID:          toolCall.ID,
// 							Name:        toolCall.Function.Name,
// 							Arguments:   make(map[string]interface{}),
// 							PartialArgs: "",
// 						}
// 						output.Content = append(output.Content, types.ToolCall{
// 							Type:      "toolCall",
// 							ID:        toolCall.ID,
// 							Name:      toolCall.Function.Name,
// 							Arguments: make(map[string]interface{}),
// 						})
// 						stream.Events <- types.EventToolcallStart{
// 							ContentIndex: blockIndex(),
// 							Partial:      output,
// 						}
// 					}
// 					// Update current tool call block
// 					if toolCallBlock, ok := currentBlock.(*ToolCallBlock); ok {
// 						if toolCall.ID != "" {
// 							toolCallBlock.ID = toolCall.ID
// 						}
// 						if toolCall.Function.Name != "" {
// 							toolCallBlock.Name = toolCall.Function.Name
// 						}
// 						deltaArgs := ""
// 						if toolCall.Function.Arguments != "" {
// 							deltaArgs = toolCall.Function.Arguments
// 							toolCallBlock.PartialArgs += toolCall.Function.Arguments
// 							// Try to parse streaming JSON (partial parsing)
// 							args := p.parseStreamingJSON(toolCallBlock.PartialArgs)
// 							toolCallBlock.Arguments = args
// 							// Update the content in output
// 							if idx := blockIndex(); idx >= 0 && idx < len(output.Content) {
// 								if tc, ok := output.Content[idx].(types.ToolCall); ok {
// 									tc.ID = toolCallBlock.ID
// 									tc.Name = toolCallBlock.Name
// 									tc.Arguments = args
// 									output.Content[idx] = tc
// 								}
// 							}
// 						}
// 						stream.Events <- types.EventToolcallDelta{
// 							ContentIndex: blockIndex(),
// 							Delta:        deltaArgs,
// 							Partial:      output,
// 						}
// 					}
// 				}
// 			}
// 			// Handle reasoning_details (for encrypted reasoning)
// 			if reasoningDetails := p.getReasoningDetails(delta); len(reasoningDetails) > 0 {
// 				for _, detail := range reasoningDetails {
// 					if detail.Type == "reasoning.encrypted" && detail.ID != "" && detail.Data != "" {
// 						// Find matching tool call and attach thought signature
// 						for i, content := range output.Content {
// 							if tc, ok := content.(types.ToolCall); ok && tc.ID == detail.ID {
// 								detailJSON, _ := json.Marshal(detail)
// 								tc.ThoughtSignature = string(detailJSON)
// 								output.Content[i] = tc
// 								break
// 							}
// 						}
// 					}
// 				}
// 			}
// 		}
// 		// Finish the last block
// 		finishCurrentBlock(currentBlock)
// 		// Check for abort
// 		if ctx.Err() != nil {
// 			p.handleError(&stream, &output, fmt.Errorf("request was aborted"), options)
// 			return
// 		}
// 		// Check for error stop reasons
// 		if output.StopReason == types.StopReasonAborted || output.StopReason == types.StopReasonError {
// 			p.handleError(&stream, &output, fmt.Errorf("an unknown error occurred"), options)
// 			return
// 		}
// 		// Send done event
// 		stream.Events <- types.EventDone{
// 			Reason:  output.StopReason,
// 			Message: output,
// 		}
// 		stream.Result <- output
// 	}()
// 	return stream
// }

// // Helper functions
// func (p *OpenAIProvider) handleError(
// 	stream *types.AssistantMessageEventStream,
// 	output *types.AssistantMessage,
// 	err error,
// 	options *types.StreamOptions,
// ) {
// 	// Determine stop reason
// 	if ctx.Err() != nil {
// 		output.StopReason = types.StopReasonAborted
// 	} else {
// 		output.StopReason = types.StopReasonError
// 	}
// 	output.ErrorMessage = err.Error()
// 	stream.Events <- types.EventError{
// 		Reason: output.StopReason,
// 		Error:  *output,
// 	}
// 	stream.Err <- err
// }
// func (p *OpenAIProvider) mapStopReason(reason string) types.StopReason {
// 	switch reason {
// 	case "stop":
// 		return types.StopReasonStop
// 	case "length":
// 		return types.StopReasonLength
// 	case "function_call", "tool_calls":
// 		return types.StopReasonToolUse
// 	case "content_filter":
// 		return types.StopReasonError
// 	default:
// 		return types.StopReasonStop
// 	}
// }
// func (p *OpenAIProvider) getReasoningField(delta interface{}, field string) string {
// 	// You'll need to implement this based on your delta structure
// 	// This should extract the reasoning field from the delta object
// 	// Return empty string if not found
// 	return ""
// }
// func (p *OpenAIProvider) getReasoningDetails(delta interface{}) []ReasoningDetail {
// 	// You'll need to implement this based on your delta structure
// 	return nil
// }
// func (p *OpenAIProvider) parseStreamingJSON(partial string) map[string]interface{} {
// 	// Attempt to parse partial JSON
// 	// If it fails, return what we can parse or empty map
// 	result := make(map[string]interface{})
// 	if err := json.Unmarshal([]byte(partial), &result); err != nil {
// 		// You might want to implement a more sophisticated partial JSON parser
// 		// For now, return empty map on parse failure
// 		return make(map[string]interface{})
// 	}
// 	return result
// }

// type ReasoningDetail struct {
// 	Type string `json:"type"`
// 	ID   string `json:"id"`
// 	Data string `json:"data"`
// }
