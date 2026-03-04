package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/providers"
)

func TestCrossProviderHandoffE2E_OpenAIConsumesAnthropicContext(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIReasoningSourceModel()
	providers.ResetAPIProviders()

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
						Type:              ai.ContentTypeThinking,
						Thinking:          "I should call the tool first.",
						ThinkingSignature: "anthropic_thinking_signature",
					},
					{
						Type:      ai.ContentTypeToolCall,
						ID:        "toolu_123",
						Name:      "double_number",
						Arguments: map[string]any{"value": 21},
					},
				},
				Provider:   "anthropic",
				API:        ai.APIAnthropicMessages,
				Model:      "claude-sonnet-4-5",
				StopReason: ai.StopReasonToolUse,
				Timestamp:  time.Now().UnixMilli(),
			},
			{
				Role:       ai.RoleToolResult,
				ToolCallID: "toolu_123",
				ToolName:   "double_number",
				Content: []ai.ContentBlock{
					{Type: ai.ContentTypeText, Text: "42"},
				},
				Timestamp: time.Now().UnixMilli(),
			},
			{
				Role:      ai.RoleAssistant,
				Content:   []ai.ContentBlock{{Type: ai.ContentTypeText, Text: "The doubled value is 42."}},
				Provider:  "anthropic",
				API:       ai.APIAnthropicMessages,
				Model:     "claude-sonnet-4-5",
				Timestamp: time.Now().UnixMilli(),
			},
			{
				Role:      ai.RoleUser,
				Text:      "Say hello to confirm handoff success.",
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
		t.Fatalf("expected non-error response, got %q", response.ErrorMessage)
	}
	text := strings.ToLower(strings.TrimSpace(firstText(response)))
	if text == "" {
		t.Fatalf("expected non-empty text response")
	}
	if !strings.Contains(text, "hello") {
		t.Fatalf("expected handoff confirmation to contain hello, got %q", text)
	}
}

func TestCrossProviderHandoffE2E_AnthropicConsumesOpenAIContext(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}
	model := anthropicE2EModel()
	providers.ResetAPIProviders()

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
						Type:              ai.ContentTypeThinking,
						Thinking:          "Need to call tool.",
						ThinkingSignature: `{"type":"reasoning","id":"rs_123","summary":[{"type":"summary_text","text":"call tool"}]}`,
					},
					{
						Type:      ai.ContentTypeToolCall,
						ID:        "call_123|fc_456",
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
				ToolCallID: "call_123|fc_456",
				ToolName:   "double_number",
				Content: []ai.ContentBlock{
					{Type: ai.ContentTypeText, Text: "42"},
				},
				Timestamp: time.Now().UnixMilli(),
			},
			{
				Role:      ai.RoleAssistant,
				Content:   []ai.ContentBlock{{Type: ai.ContentTypeText, Text: "The doubled value is 42."}},
				Provider:  "openai",
				API:       ai.APIOpenAIResponses,
				Model:     "gpt-5-mini",
				Timestamp: time.Now().UnixMilli(),
			},
			{
				Role:      ai.RoleUser,
				Text:      "Say hello to confirm handoff success.",
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
		t.Fatalf("expected non-error response, got %q", response.ErrorMessage)
	}
	text := strings.ToLower(strings.TrimSpace(firstText(response)))
	if text == "" {
		t.Fatalf("expected non-empty text response")
	}
	if !strings.Contains(text, "hello") {
		t.Fatalf("expected handoff confirmation to contain hello, got %q", text)
	}
}
