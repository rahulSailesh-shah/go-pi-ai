package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rahulSailesh-shah/go-pi-ai/provider"
	"github.com/rahulSailesh-shah/go-pi-ai/types"
)

func main() {
	model := types.Model{
		Provider: types.ProviderNvidia,
		ID:       "openai/gpt-oss-20b",
	}

	log.Printf("Using model: %s from provider: %s\n", model.ID, model.Provider)

	tools := []types.Tool{
		{
			Name:        "getWeather",
			Description: "Get the weather for a given location",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]string{"type": "string"},
				},
				"required": []string{"location"},
			},
		},
	}

	conversation := types.Context{
		SystemPrompt: "You are a helpful assistant. If you call a tool, include the results in your response.",
		Messages: []types.Message{
			types.UserMessage{
				Timestamp: time.Now(),
				Contents: []types.Content{
					types.TextContent{Text: "Write a short poem about cats. Then check weather for Tokyo and incorporate it into your response."},
				},
			},
		},
		Tools: tools,
	}

	log.Println("Starting streaming conversation...")
	stream, err := provider.Stream(context.Background(), model, conversation)
	if err != nil {
		log.Fatalf("Streaming failed: %v", err)
	}

	// Drain events
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range stream.Events {
			switch e := event.(type) {
			case types.EventTextDelta:
				log.Printf("Text: %s", e.Delta)
			case types.EventToolcallStart:
				log.Println("Tool call started")
			case types.EventDone:
				log.Println("Stream completed")
			}
		}
	}()

	log.Println("Waiting for streaming result...")
	finalMessage := <-stream.Result
	wg.Wait() // Wait for all events to be processed
	if err := <-stream.Err; err != nil {
		log.Printf("Streaming error: %v", err)
	}

	log.Println("Streaming complete")
	conversation.Messages = append(conversation.Messages, finalMessage)

	toolCalls := []types.ToolCall{}
	for _, content := range finalMessage.Contents {
		if tc, ok := content.(types.ToolCall); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	if len(toolCalls) > 0 {
		log.Printf("Tool calls detected: %d", len(toolCalls))
		for _, toolCall := range toolCalls {
			log.Printf("Tool: %s - executing...", toolCall.Name)

			conversation.Messages = append(conversation.Messages, types.ToolMessage{
				ToolCallId: toolCall.ID,
				ToolName:   toolCall.Name,
				Timestamp:  time.Now(),
				Contents: []types.Content{
					types.TextContent{Text: "Weather in Tokyo, Japan: 72°F (22°C), partly cloudy"},
				},
			})
		}
	}

	log.Println("Starting completion call...")
	completeMessage, err := provider.Complete(context.Background(), model, conversation)
	if err != nil {
		log.Fatalf("Completion failed: %v", err)
	}

	log.Printf("\nFinal response:\n%s\n", formatContent(completeMessage.Contents))
}

func formatContent(contents []types.Content) string {
	var result string
	for _, content := range contents {
		switch c := content.(type) {
		case types.TextContent:
			result += c.Text + "\n\n---\n\n"
		case types.ToolCall:
			result += fmt.Sprintf("[Tool Call: %s]\n", c.Name)
		}
	}
	return result
}
