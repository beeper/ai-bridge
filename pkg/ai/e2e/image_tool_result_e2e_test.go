package e2e

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/providers"
)

// 1x1 red PNG.
const redPixelBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADUlEQVR42mP8z8BQDwAE/wH+1J1iGQAAAABJRU5ErkJggg=="

func TestImageToolResultE2E_OpenAI(t *testing.T) {
	requirePIAIE2E(t)
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := openAIE2EModel()
	model.Input = []string{"text", "image"}
	providers.ResetAPIProviders()

	testCases := []struct {
		name          string
		toolResult    []ai.ContentBlock
		prompt        string
		expectKeyword string
	}{
		{
			name: "image-only-tool-result",
			toolResult: []ai.ContentBlock{
				{Type: ai.ContentTypeImage, Data: redPixelBase64, MimeType: "image/png"},
			},
			prompt:        "Describe what you see in the tool result image. Mention the color.",
			expectKeyword: "red",
		},
		{
			name: "text-and-image-tool-result",
			toolResult: []ai.ContentBlock{
				{Type: ai.ContentTypeText, Text: "The shape has a diameter of 100 pixels."},
				{Type: ai.ContentTypeImage, Data: redPixelBase64, MimeType: "image/png"},
			},
			prompt:        "Summarize the tool result details and mention any visible color.",
			expectKeyword: "pixel",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			context := ai.Context{
				SystemPrompt: "You are a helpful assistant.",
				Tools:        []ai.Tool{doubleNumberTool()},
				Messages: []ai.Message{
					{
						Role:      ai.RoleUser,
						Text:      "Use the tool result to answer the next question.",
						Timestamp: time.Now().UnixMilli(),
					},
					{
						Role: ai.RoleAssistant,
						Content: []ai.ContentBlock{
							{
								Type:      ai.ContentTypeToolCall,
								ID:        "call_123|fc_456",
								Name:      "double_number",
								Arguments: map[string]any{"value": 21},
							},
						},
						Provider:   model.Provider,
						API:        model.API,
						Model:      model.ID,
						StopReason: ai.StopReasonToolUse,
						Timestamp:  time.Now().UnixMilli(),
					},
					{
						Role:       ai.RoleToolResult,
						ToolCallID: "call_123|fc_456",
						ToolName:   "double_number",
						Content:    tc.toolResult,
						Timestamp:  time.Now().UnixMilli(),
					},
					{
						Role:      ai.RoleUser,
						Text:      tc.prompt,
						Timestamp: time.Now().UnixMilli(),
					},
				},
			}

			response, err := ai.CompleteSimple(model, context, &ai.SimpleStreamOptions{
				StreamOptions: ai.StreamOptions{
					APIKey:    apiKey,
					MaxTokens: 512,
				},
				Reasoning: ai.ThinkingMedium,
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
			if !strings.Contains(text, tc.expectKeyword) && !strings.Contains(text, "red") {
				t.Fatalf("expected response to reference tool result content, got %q", text)
			}
		})
	}
}
