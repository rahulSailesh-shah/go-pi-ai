package main

import (
	"ai/pkg/config"
	"ai/pkg/models"
	"ai/pkg/types"
	"context"
	"encoding/json"
)

func main() {
	nvidiaConfig, ok := config.GetProvider(types.ApiNvidia)
	if !ok {
		panic("provider not found")
	}

	model := models.GetModel(types.ApiNvidia, nvidiaConfig.Models[0])
	if model == nil {
		panic("model not found")
	}

	conversation := types.Context{
		SystemPrompt: "You are a helpful assistant that can answer questions and help with tasks.",
		Messages: []types.Message{
			types.UserMessage{
				Contents: []types.Content{
					types.TextContent{
						Text: "Write me a poem that rhymes with the word 'cat', and then call the getWeather tool to get the weather for the city of Tokyo, Japan.",
					},
				},
			},
		},
		Tools: []types.Tool{
			{
				Name:        "getWeather",
				Description: "Get the weather for a given location",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]string{
							"type": "string",
						},
					},
					"required": []string{"location"},
				},
			},
			{
				Name:        "getPopulation",
				Description: "Get the population for a given location",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"town": map[string]string{
							"type": "string",
						},
						"nation": map[string]string{
							"type": "string",
						},
						"rounding": map[string]string{
							"type":        "integer",
							"description": "Nearest base 10 to round to, e.g. 1000 or 1000000",
						},
					},
					"required": []string{"town", "nation"},
				},
			},
		},
	}

	stream := model.Stream(context.Background(), conversation)

	// Drain events in background
	go func() {
		for range stream.Events {
		}
	}()

	finalMessage := <-stream.Result
	conversation.Messages = append(conversation.Messages, finalMessage)

	toolCalls := []types.ToolCall{}
	for _, content := range finalMessage.Contents {
		if content.Type() == "toolCall" {
			toolCalls = append(toolCalls, content.(types.ToolCall))
		}
	}

	println("toolCalls: ", len(toolCalls))

	if len(toolCalls) > 0 {
		for _, toolCall := range toolCalls {
			conversation.Messages = append(conversation.Messages, types.ToolMessage{
				ToolCallId: toolCall.ID,
				ToolName:   toolCall.Name,
				Contents: []types.Content{
					types.TextContent{
						Text: "The weather in Tokyo, Japan is 20 degrees Celsius.",
					},
				},
			})
		}
	}

	completeMessage, err := model.Complete(context.Background(), conversation)
	if err != nil {
		println(err.Error())
		return
	}

	data, _ := json.MarshalIndent(completeMessage, "", "  ")
	println(string(data))
}
