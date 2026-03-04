package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/providers"
)

func TestEmptyE2E_OpenAI(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIE2EModel()
	providers.ResetAPIProviders()

	t.Run("empty-content-array", func(t *testing.T) {
		response := completeOpenAISimple(t, model, apiKey, ai.Context{
			Messages: []ai.Message{
				{
					Role:      ai.RoleUser,
					Content:   []ai.ContentBlock{},
					Timestamp: time.Now().UnixMilli(),
				},
			},
		})
		assertGracefulEmptyResponse(t, response)
	})

	t.Run("empty-string", func(t *testing.T) {
		response := completeOpenAISimple(t, model, apiKey, ai.Context{
			Messages: []ai.Message{
				{
					Role:      ai.RoleUser,
					Text:      "",
					Timestamp: time.Now().UnixMilli(),
				},
			},
		})
		assertGracefulEmptyResponse(t, response)
	})

	t.Run("whitespace-only", func(t *testing.T) {
		response := completeOpenAISimple(t, model, apiKey, ai.Context{
			Messages: []ai.Message{
				{
					Role:      ai.RoleUser,
					Text:      "   \n\t  ",
					Timestamp: time.Now().UnixMilli(),
				},
			},
		})
		assertGracefulEmptyResponse(t, response)
	})

	t.Run("empty-assistant-in-history", func(t *testing.T) {
		response := completeOpenAISimple(t, model, apiKey, ai.Context{
			Messages: []ai.Message{
				{
					Role:      ai.RoleUser,
					Text:      "Hello, how are you?",
					Timestamp: time.Now().UnixMilli(),
				},
				{
					Role:       ai.RoleAssistant,
					Content:    []ai.ContentBlock{},
					API:        model.API,
					Provider:   model.Provider,
					Model:      model.ID,
					StopReason: ai.StopReasonStop,
					Usage: ai.Usage{
						Input:       10,
						Output:      0,
						CacheRead:   0,
						CacheWrite:  0,
						TotalTokens: 10,
					},
					Timestamp: time.Now().UnixMilli(),
				},
				{
					Role:      ai.RoleUser,
					Text:      "Please respond this time.",
					Timestamp: time.Now().UnixMilli(),
				},
			},
		})
		assertGracefulEmptyResponse(t, response)
		if response.StopReason != ai.StopReasonError && len(response.Content) == 0 {
			t.Fatalf("expected non-empty assistant response content")
		}
	})
}

func completeOpenAISimple(t *testing.T, model ai.Model, apiKey string, context ai.Context) ai.Message {
	t.Helper()
	response, err := ai.CompleteSimple(model, context, &ai.SimpleStreamOptions{
		StreamOptions: ai.StreamOptions{
			APIKey:    apiKey,
			MaxTokens: 256,
		},
		Reasoning: ai.ThinkingMedium,
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	return response
}

func assertGracefulEmptyResponse(t *testing.T, response ai.Message) {
	t.Helper()
	if response.Role != ai.RoleAssistant {
		t.Fatalf("expected assistant role, got %q", response.Role)
	}
	if response.StopReason == ai.StopReasonError {
		if strings.TrimSpace(response.ErrorMessage) == "" {
			t.Fatalf("expected non-empty error message for error response")
		}
		return
	}
	if response.Content == nil {
		t.Fatalf("expected content to be initialized")
	}
}
