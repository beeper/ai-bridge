package connector

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPkgAIProviderBridgeE2E_CompleteOpenAI(t *testing.T) {
	requirePkgAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := envOrDefault("PI_AI_E2E_OPENAI_MODEL", "gpt-4o-mini")

	resp, handled, err := tryGenerateWithPkgAI(context.Background(), "", apiKey, GenerateParams{
		Model: model,
		Messages: []UnifiedMessage{
			{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeText, Text: "Reply with the single word OK."}}},
		},
		MaxCompletionTokens: 128,
	})
	if err != nil {
		t.Fatalf("expected successful pkg/ai completion, got error: %v", err)
	}
	if !handled {
		t.Fatalf("expected pkg/ai bridge to handle OpenAI completion")
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		t.Fatalf("expected non-empty completion response")
	}
}

func TestPkgAIProviderBridgeE2E_CompleteAnthropic(t *testing.T) {
	requirePkgAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}
	model := envOrDefault("PI_AI_E2E_ANTHROPIC_MODEL", "claude-3-5-haiku-latest")

	resp, handled, err := tryGenerateWithPkgAI(context.Background(), "https://api.anthropic.com", apiKey, GenerateParams{
		Model: model,
		Messages: []UnifiedMessage{
			{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeText, Text: "Reply with the single word OK."}}},
		},
		MaxCompletionTokens: 128,
	})
	if err != nil {
		t.Fatalf("expected successful pkg/ai completion, got error: %v", err)
	}
	if !handled {
		t.Fatalf("expected pkg/ai bridge to handle Anthropic completion")
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		t.Fatalf("expected non-empty completion response")
	}
}

func TestPkgAIProviderBridgeE2E_CompleteGoogle(t *testing.T) {
	requirePkgAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is not set")
	}
	model := envOrDefault("PI_AI_E2E_GOOGLE_MODEL", "gemini-2.5-flash")

	resp, handled, err := tryGenerateWithPkgAI(context.Background(), "https://generativelanguage.googleapis.com", apiKey, GenerateParams{
		Model: model,
		Messages: []UnifiedMessage{
			{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeText, Text: "Reply with the single word OK."}}},
		},
		MaxCompletionTokens: 128,
	})
	if err != nil {
		t.Fatalf("expected successful pkg/ai completion, got error: %v", err)
	}
	if !handled {
		t.Fatalf("expected pkg/ai bridge to handle Google completion")
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		t.Fatalf("expected non-empty completion response")
	}
}

func TestPkgAIProviderBridgeE2E_StreamOpenAI(t *testing.T) {
	requirePkgAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := envOrDefault("PI_AI_E2E_OPENAI_MODEL", "gpt-4o-mini")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	events, handled := tryGenerateStreamWithPkgAI(ctx, "", apiKey, GenerateParams{
		Model: model,
		Messages: []UnifiedMessage{
			{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeText, Text: "Reply with the single word OK."}}},
		},
		MaxCompletionTokens: 128,
	})
	if !handled {
		t.Fatalf("expected pkg/ai stream bridge to handle OpenAI streaming")
	}

	receivedDelta := false
	receivedComplete := false
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for stream completion")
		case evt, ok := <-events:
			if !ok {
				if !receivedComplete {
					t.Fatalf("stream closed before complete event")
				}
				return
			}
			switch evt.Type {
			case StreamEventDelta:
				if strings.TrimSpace(evt.Delta) != "" {
					receivedDelta = true
				}
			case StreamEventComplete:
				receivedComplete = true
				if !receivedDelta {
					t.Fatalf("expected at least one text delta before completion")
				}
				return
			case StreamEventError:
				t.Fatalf("unexpected stream error: %v", evt.Error)
			}
		}
	}
}

func requirePkgAIE2E(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("PI_AI_E2E")) != "1" {
		t.Skip("set PI_AI_E2E=1 to run connector pkg/ai bridge e2e tests")
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
