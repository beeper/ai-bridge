package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/providers"
)

func TestXhighE2E_OpenAIResponses(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIReasoningTargetModel()
	providers.ResetAPIProviders()

	response, err := ai.CompleteSimple(model, ai.Context{
		Messages: []ai.Message{
			{
				Role:      ai.RoleUser,
				Text:      "Think step by step and then answer: what is 17 + 25?",
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}, &ai.SimpleStreamOptions{
		StreamOptions: ai.StreamOptions{
			APIKey:    apiKey,
			MaxTokens: 256,
		},
		Reasoning: ai.ThinkingXHigh,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if response.StopReason == ai.StopReasonError {
		lower := strings.ToLower(response.ErrorMessage)
		if strings.Contains(lower, "model") &&
			(strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") || strings.Contains(lower, "access")) {
			t.Skipf("model not available for this API key: %s", response.ErrorMessage)
		}
		t.Fatalf("expected non-error response, got %q", response.ErrorMessage)
	}
	text := strings.ToLower(strings.TrimSpace(firstText(response)))
	if text == "" {
		t.Fatalf("expected non-empty text response")
	}
	if !strings.Contains(text, "42") &&
		!strings.Contains(text, "forty-two") &&
		!strings.Contains(text, "forty two") {
		t.Fatalf("expected computed answer in response, got %q", text)
	}
}
