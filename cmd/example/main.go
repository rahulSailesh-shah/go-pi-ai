package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	gopiai "github.com/rahulSailesh-shah/go-pi-ai"
	"github.com/rahulSailesh-shah/go-pi-ai/openai"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	provider, err := openai.NewProvider(openai.Config{
		APIKey:  os.Getenv("NVIDIA_API_KEY"),
		BaseURL: "https://integrate.api.nvidia.com/v1",
	})
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}

	client := gopiai.NewClient(provider)

	tools := []gopiai.Tool{
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

	req := gopiai.Request{
		Model:        "openai/gpt-oss-20b",
		SystemPrompt: "You are a helpful assistant. If you call a tool, include the results in your response.",
		Messages: []gopiai.Message{
			gopiai.UserMessage{
				Timestamp: time.Now(),
				Contents: []gopiai.Content{
					gopiai.TextContent{Text: "Write a short poem about cats. Then check weather for Tokyo and incorporate it into your response."},
				},
			},
		},
		Tools: tools,
	}

	// --- Complete example (tool call expected) ---
	log.Println("Starting completion...")
	firstMessage, err := client.Complete(context.Background(), req)
	if err != nil {
		log.Fatalf("Completion failed: %v", err)
	}
	log.Printf("Stop reason: %s", firstMessage.StopReason)

	// --- Multi-turn with tool results ---
	req.Messages = append(req.Messages, firstMessage)

	var toolCalls []gopiai.ToolCall
	for _, content := range firstMessage.Contents {
		if tc, ok := content.(gopiai.ToolCall); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	if len(toolCalls) > 0 {
		log.Printf("Tool calls detected: %d", len(toolCalls))
		for _, toolCall := range toolCalls {
			log.Printf("Tool: %s - executing...", toolCall.Name)

			req.Messages = append(req.Messages, gopiai.ToolMessage{
				ToolCallID: toolCall.ID,
				ToolName:   toolCall.Name,
				Timestamp:  time.Now(),
				Contents: []gopiai.Content{
					gopiai.TextContent{Text: "Weather in Tokyo, Japan: 72°F (22°C), partly cloudy"},
				},
			})
		}
	}

	log.Println("Starting completion call...")
	stream2, err := client.Stream(context.Background(), req)
	if err != nil {
		log.Fatalf("Streaming failed: %v", err)
	}
	defer stream2.Close()

	fmt.Println()
	var streamResult gopiai.AssistantMessage
	for {
		event, err := stream2.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Stream error: %v", err)
		}
		switch e := event.(type) {
		case gopiai.EventTextDelta:
			fmt.Print(e.Delta)
		case gopiai.EventDone:
			streamResult = e.Message
		}
	}

	fmt.Println("\n\n--------------------------------")
	finalJson, _ := json.MarshalIndent(streamResult, "", "  ")
	fmt.Println(string(finalJson))

}
