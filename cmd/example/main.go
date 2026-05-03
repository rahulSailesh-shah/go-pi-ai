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

func saveResponseToFile(filename string, msg gopiai.Message) {
	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}
	if err := os.WriteFile(filename, data, 0644); err != nil {
		log.Printf("Failed to save response to file: %v", err)
		return
	}
	fmt.Printf("\nSaved response to %s\n", filename)
}

func main() {
	godotenv.Load()
	provider, err := openai.NewProvider(openai.Config{
		APIKey:  os.Getenv("NVIDIA_API_KEY"),
		BaseURL: os.Getenv("NVIDIA_BASE_URL"),
	})
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}

	client := gopiai.NewClient(provider)
	fmt.Println("=== Example 1: Simple Complete ===")
	simpleCompleteExample(client)

	fmt.Println("\n=== Example 2: Simple Streaming ===")
	simpleStreamingExample(client)

	fmt.Println("\n=== Example 3: Tool Calling ===")
	toolCallingExample(client)
}

func simpleCompleteExample(client *gopiai.Client) {
	req := gopiai.Request{
		Model: "openai/gpt-oss-120b",
		Messages: []gopiai.Message{
			gopiai.UserMessage{
				Timestamp: time.Now(),
				Contents: []gopiai.Content{
					gopiai.TextContent{Text: "Write a short poem about APIs"},
				},
			},
		},
	}
	fmt.Println("Response:")
	fmt.Println("---")

	ctx := context.Background()
	msg, err := client.Complete(ctx, req)
	if err != nil {
		log.Fatalf("Failed to complete: %v", err)
	}

	for _, c := range msg.Contents {
		if text, ok := c.(gopiai.TextContent); ok {
			fmt.Print(text.Text)
		}
	}
	fmt.Println("\n---")

	saveResponseToFile("complete_response.json", msg)
}

func simpleStreamingExample(client *gopiai.Client) {
	req := gopiai.Request{
		Model: "openai/gpt-oss-120b",
		Messages: []gopiai.Message{
			gopiai.UserMessage{
				Timestamp: time.Now(),
				Contents: []gopiai.Content{
					gopiai.TextContent{Text: "Write a haiku about programming"},
				},
			},
		},
	}
	fmt.Println("Response:")
	fmt.Println("---")

	ctx := context.Background()
	stream, err := client.Stream(ctx, req)
	if err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}
	defer stream.Close()

	var finalMsg gopiai.AssistantMessage
	for {
		event, err := stream.Recv()
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
			finalMsg = e.Message
			if e.Err != nil {
				log.Printf("Stream ended with error: %v", e.Err)
			}
		}
	}

	fmt.Println("\n---")
	saveResponseToFile("stream_response.json", finalMsg)
}

func toolCallingExample(client *gopiai.Client) {
	tools := []gopiai.Tool{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a location",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]string{
						"type":        "string",
						"description": "The city name, e.g. San Francisco",
					},
				},
				"required": []string{"location"},
			},
		},
	}

	req := gopiai.Request{
		Model: "openai/gpt-oss-120b",
		Messages: []gopiai.Message{
			gopiai.UserMessage{
				Timestamp: time.Now(),
				Contents: []gopiai.Content{
					gopiai.TextContent{Text: "What's the weather in Tokyo?"},
				},
			},
		},
		Tools: tools,
	}

	fmt.Println("First response (AI decides to call tool):")
	fmt.Println("---")

	ctx := context.Background()
	stream, err := client.Stream(ctx, req)
	if err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}
	defer stream.Close()

	var assistantMessage gopiai.AssistantMessage
	for {
		event, err := stream.Recv()
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
			assistantMessage = e.Message
			if e.Err != nil {
				log.Printf("Stream ended with error: %v", e.Err)
			}
		}
	}

	fmt.Println("\n---")

	var toolCalls []gopiai.ToolCall
	for _, content := range assistantMessage.Contents {
		if tc, ok := content.(gopiai.ToolCall); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	if len(toolCalls) == 0 {
		fmt.Println("No tool calls were made")
		return
	}

	fmt.Printf("\nExecuting %d tool call(s):\n", len(toolCalls))
	for _, toolCall := range toolCalls {
		fmt.Printf("  Tool: %s\n", toolCall.Name)
		fmt.Printf("  Arguments: %s\n", toolCall.RawArguments)

		var args map[string]any
		json.Unmarshal([]byte(toolCall.RawArguments), &args)
		location := args["location"].(string)

		toolResult := fmt.Sprintf("Weather in %s: 72°F (22°C), sunny", location)

		req.Messages = append(req.Messages, assistantMessage)
		req.Messages = append(req.Messages, gopiai.ToolMessage{
			ToolCallID: toolCall.ID,
			ToolName:   toolCall.Name,
			Timestamp:  time.Now(),
			Contents: []gopiai.Content{
				gopiai.TextContent{Text: toolResult},
			},
		})
	}

	fmt.Println("\nFinal response (AI uses tool result):")
	fmt.Println("---")

	stream2, err := client.Stream(context.Background(), req)
	if err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}
	defer stream2.Close()

	var finalMsg gopiai.AssistantMessage
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
			finalMsg = e.Message
			if e.Err != nil {
				log.Printf("Stream ended with error: %v", e.Err)
			}
		}
	}

	fmt.Println("\n---")
	saveResponseToFile("tool_calling_response.json", finalMsg)
}
