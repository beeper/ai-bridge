package providers

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestStreamGoogleGeminiCLI_MissingAPIKeyEmitsError(t *testing.T) {
	stream := streamGoogleGeminiCLI(ai.Model{
		ID:       "gemini-2.5-flash",
		Provider: "google-gemini-cli",
		API:      ai.APIGoogleGeminiCLI,
	}, ai.Context{
		Messages: []ai.Message{{Role: ai.RoleUser, Text: "hello"}},
	}, &ai.StreamOptions{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	evt, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("expected terminal error event, got %v", err)
	}
	if evt.Type != ai.EventError {
		t.Fatalf("expected error event, got %s", evt.Type)
	}
	if !strings.Contains(strings.ToLower(evt.Error.ErrorMessage), "oauth") {
		t.Fatalf("expected oauth auth message, got %q", evt.Error.ErrorMessage)
	}
	if _, err := stream.Next(ctx); err != io.EOF {
		t.Fatalf("expected EOF after terminal event, got %v", err)
	}
}

func TestStreamGoogleGeminiCLI_InvalidAPIKeyEmitsError(t *testing.T) {
	stream := streamGoogleGeminiCLI(ai.Model{
		ID:       "gemini-2.5-flash",
		Provider: "google-gemini-cli",
		API:      ai.APIGoogleGeminiCLI,
	}, ai.Context{
		Messages: []ai.Message{{Role: ai.RoleUser, Text: "hello"}},
	}, &ai.StreamOptions{
		APIKey: "not-json",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	evt, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("expected terminal error event, got %v", err)
	}
	if evt.Type != ai.EventError {
		t.Fatalf("expected error event, got %s", evt.Type)
	}
	if !strings.Contains(strings.ToLower(evt.Error.ErrorMessage), "invalid google cloud credentials") {
		t.Fatalf("expected invalid credentials message, got %q", evt.Error.ErrorMessage)
	}
}

func TestBuildGoogleGeminiCLIRequest(t *testing.T) {
	temp := 0.4
	request := BuildGoogleGeminiCLIRequest(
		ai.Model{ID: "gemini-2.5-flash", Reasoning: true},
		ai.Context{
			SystemPrompt: "You are helpful",
			Messages: []ai.Message{
				{Role: ai.RoleUser, Text: "hello"},
			},
			Tools: []ai.Tool{
				{Name: "search", Description: "search tool", Parameters: map[string]any{"type": "object"}},
			},
		},
		"project-123",
		googleGeminiCLIOptions{
			StreamOptions: ai.StreamOptions{
				Temperature: &temp,
				MaxTokens:   2048,
				SessionID:   "session-1",
			},
			ToolChoice: "any",
			Thinking: &GoogleThinkingOptions{
				Enabled: true,
				Level:   "HIGH",
			},
		},
		false,
	)

	if request["project"] != "project-123" {
		t.Fatalf("expected project mapping, got %#v", request["project"])
	}
	if request["model"] != "gemini-2.5-flash" {
		t.Fatalf("expected model mapping, got %#v", request["model"])
	}
	req, ok := request["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested request object, got %#v", request["request"])
	}
	if req["sessionId"] != "session-1" {
		t.Fatalf("expected session id mapping, got %#v", req["sessionId"])
	}
	if _, ok := req["contents"]; !ok {
		t.Fatalf("expected converted contents in request")
	}
	if _, ok := req["systemInstruction"]; !ok {
		t.Fatalf("expected system instruction object")
	}
	genCfg, ok := req["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected generationConfig object, got %#v", req["generationConfig"])
	}
	if genCfg["maxOutputTokens"] != 2048 {
		t.Fatalf("expected maxOutputTokens mapping, got %#v", genCfg["maxOutputTokens"])
	}
	if genCfg["temperature"] != 0.4 {
		t.Fatalf("expected temperature mapping, got %#v", genCfg["temperature"])
	}
}

func TestGeminiCLIThinkingLevel(t *testing.T) {
	if got := getGeminiCLIThinkingLevel(ai.ThinkingMinimal, "gemini-3-pro"); got != "LOW" {
		t.Fatalf("expected pro minimal -> LOW, got %q", got)
	}
	if got := getGeminiCLIThinkingLevel(ai.ThinkingMedium, "gemini-3-pro"); got != "HIGH" {
		t.Fatalf("expected pro medium -> HIGH, got %q", got)
	}
	if got := getGeminiCLIThinkingLevel(ai.ThinkingMedium, "gemini-3-flash"); got != "MEDIUM" {
		t.Fatalf("expected flash medium -> MEDIUM, got %q", got)
	}
}
