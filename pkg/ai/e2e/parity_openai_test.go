package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/providers"
)

func TestToolCallWithoutResultE2E_OpenAI(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIE2EModel()
	providers.ResetAPIProviders()

	tool := ai.Tool{
		Name:        "calculate",
		Description: "Evaluate math expressions",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{"type": "string"},
			},
			"required": []any{"expression"},
		},
	}
	context := ai.Context{
		SystemPrompt: "Use the calculate tool for arithmetic operations.",
		Tools:        []ai.Tool{tool},
		Messages: []ai.Message{
			{
				Role:      ai.RoleUser,
				Text:      "Please calculate 25 * 18 using the calculate tool.",
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}

	first, err := ai.Complete(model, context, &ai.StreamOptions{
		APIKey:    apiKey,
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("first complete failed: %v", err)
	}
	toolCall, ok := findFirstToolCall(first)
	if !ok {
		t.Fatalf("expected tool call in first response, stop=%q err=%q", first.StopReason, first.ErrorMessage)
	}

	context.Messages = append(context.Messages, first)
	context.Messages = append(context.Messages, ai.Message{
		Role:      ai.RoleUser,
		Text:      "Never mind; just tell me what is 2+2?",
		Timestamp: time.Now().UnixMilli(),
	})

	second, err := ai.Complete(model, context, &ai.StreamOptions{
		APIKey:    apiKey,
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("second complete failed: %v", err)
	}
	if second.StopReason == ai.StopReasonError {
		t.Fatalf("expected non-error response after orphan tool call, got %q", second.ErrorMessage)
	}
	if len(second.Content) == 0 {
		t.Fatalf("expected non-empty response content")
	}
	if second.StopReason != ai.StopReasonStop && second.StopReason != ai.StopReasonToolUse {
		t.Fatalf("unexpected stop reason %q", second.StopReason)
	}
	if second.StopReason == ai.StopReasonToolUse {
		if _, hasTool := findFirstToolCall(second); !hasTool {
			t.Fatalf("expected toolUse response to include a tool call")
		}
	}
	if strings.TrimSpace(toolCall.ID) == "" {
		t.Fatalf("expected first tool call id to be populated")
	}
}

func TestTotalTokensE2E_OpenAI(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIE2EModel()
	providers.ResetAPIProviders()

	longSystemPrompt := strings.Repeat(
		"You are a concise assistant. Include only the requested answer.\n",
		60,
	)
	context := ai.Context{
		SystemPrompt: longSystemPrompt,
		Messages: []ai.Message{
			{
				Role:      ai.RoleUser,
				Text:      "What is 2 + 2? Reply with one token only.",
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}

	first, err := ai.Complete(model, context, &ai.StreamOptions{
		APIKey:    apiKey,
		MaxTokens: 128,
	})
	if err != nil {
		t.Fatalf("first complete failed: %v", err)
	}
	assertTotalTokensEqualsComponents(t, first.Usage)

	context.Messages = append(context.Messages, first)
	context.Messages = append(context.Messages, ai.Message{
		Role:      ai.RoleUser,
		Text:      "Now what is 3 + 3? Reply with one token only.",
		Timestamp: time.Now().UnixMilli(),
	})
	second, err := ai.Complete(model, context, &ai.StreamOptions{
		APIKey:    apiKey,
		MaxTokens: 128,
	})
	if err != nil {
		t.Fatalf("second complete failed: %v", err)
	}
	assertTotalTokensEqualsComponents(t, second.Usage)
}

func findFirstToolCall(message ai.Message) (ai.ContentBlock, bool) {
	for _, block := range message.Content {
		if block.Type == ai.ContentTypeToolCall {
			return block, true
		}
	}
	return ai.ContentBlock{}, false
}

func assertTotalTokensEqualsComponents(t *testing.T, usage ai.Usage) {
	t.Helper()
	computed := usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	if usage.TotalTokens != computed {
		t.Fatalf(
			"total tokens mismatch: got %d want %d (input=%d output=%d cacheRead=%d cacheWrite=%d)",
			usage.TotalTokens,
			computed,
			usage.Input,
			usage.Output,
			usage.CacheRead,
			usage.CacheWrite,
		)
	}
}
