package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/providers"
)

func TestToolCallIDNormalizationE2E_OpenAI(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIReasoningSourceModel()
	providers.ResetAPIProviders()

	rawToolCallID := "call_abc|item+/=="
	context := ai.Context{
		SystemPrompt: "You are a helpful assistant.",
		Tools:        []ai.Tool{doubleNumberTool()},
		Messages: []ai.Message{
			{
				Role:      ai.RoleUser,
				Text:      "Use the tool to double 21.",
				Timestamp: time.Now().UnixMilli(),
			},
			{
				Role: ai.RoleAssistant,
				Content: []ai.ContentBlock{
					{
						Type:      ai.ContentTypeToolCall,
						ID:        rawToolCallID,
						Name:      "double_number",
						Arguments: map[string]any{"value": 21},
					},
				},
				Provider:   "openai",
				API:        ai.APIOpenAIResponses,
				Model:      "gpt-5-mini",
				StopReason: ai.StopReasonToolUse,
				Timestamp:  time.Now().UnixMilli(),
			},
			{
				Role:       ai.RoleToolResult,
				ToolCallID: rawToolCallID,
				ToolName:   "double_number",
				Content: []ai.ContentBlock{
					{Type: ai.ContentTypeText, Text: "42"},
				},
				Timestamp: time.Now().UnixMilli(),
			},
			{
				Role:      ai.RoleUser,
				Text:      "What was the result? Answer with just the number.",
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}

	response, err := ai.CompleteSimple(model, context, &ai.SimpleStreamOptions{
		StreamOptions: ai.StreamOptions{
			APIKey:    apiKey,
			MaxTokens: 256,
		},
		Reasoning: ai.ThinkingHigh,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if response.StopReason == ai.StopReasonError {
		t.Fatalf("expected non-error response after normalization, got %q", response.ErrorMessage)
	}
	text := strings.ToLower(strings.TrimSpace(firstText(response)))
	if text == "" {
		t.Fatalf("expected non-empty text response")
	}
	if !strings.Contains(text, "42") &&
		!strings.Contains(text, "forty-two") &&
		!strings.Contains(text, "forty two") {
		t.Fatalf("expected response to reference tool result, got %q", text)
	}
}
