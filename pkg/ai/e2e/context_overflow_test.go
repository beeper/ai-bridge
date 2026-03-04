package e2e

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/providers"
	aiutils "github.com/beeper/ai-bridge/pkg/ai/utils"
)

func TestContextOverflowE2E_OpenAI(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIE2EModel()
	model.ContextWindow = openAIE2EContextWindow()
	if model.ContextWindow <= 0 {
		t.Skip("model context window is unknown")
	}
	providers.ResetAPIProviders()

	overflowContent := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", (model.ContextWindow+10000)/10)
	response, err := ai.Complete(model, ai.Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []ai.Message{
			{
				Role:      ai.RoleUser,
				Text:      overflowContent,
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}, &ai.StreamOptions{
		APIKey:    apiKey,
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	if !aiutils.IsContextOverflow(response, model.ContextWindow) {
		t.Fatalf(
			"expected context overflow detection for stop=%q err=%q input=%d cacheRead=%d window=%d",
			response.StopReason,
			response.ErrorMessage,
			response.Usage.Input,
			response.Usage.CacheRead,
			model.ContextWindow,
		)
	}
}

func openAIE2EContextWindow() int {
	if raw := strings.TrimSpace(os.Getenv("PI_AI_E2E_OPENAI_CONTEXT_WINDOW")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			return v
		}
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(os.Getenv("PI_AI_E2E_OPENAI_MODEL"))), "gpt-5") {
		return 400000
	}
	return 128000
}
