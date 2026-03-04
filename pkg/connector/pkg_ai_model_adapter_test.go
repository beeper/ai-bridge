package connector

import (
	"testing"

	aipkg "github.com/beeper/ai-bridge/pkg/ai"
)

func TestDerivePkgAIAPI(t *testing.T) {
	cases := []struct {
		provider string
		modelAPI ModelAPI
		want     aipkg.Api
	}{
		{provider: "anthropic", modelAPI: ModelAPIResponses, want: aipkg.APIAnthropicMessages},
		{provider: "google", modelAPI: ModelAPIResponses, want: aipkg.APIGoogleGenerativeAI},
		{provider: "google-vertex", modelAPI: ModelAPIResponses, want: aipkg.APIGoogleVertex},
		{provider: "google-gemini-cli", modelAPI: ModelAPIResponses, want: aipkg.APIGoogleGeminiCLI},
		{provider: "google-antigravity", modelAPI: ModelAPIResponses, want: aipkg.APIGoogleGeminiCLI},
		{provider: "azure-openai-responses", modelAPI: ModelAPIResponses, want: aipkg.APIAzureOpenAIResponse},
		{provider: "amazon-bedrock", modelAPI: ModelAPIResponses, want: aipkg.APIBedrockConverse},
		{provider: "openai-codex", modelAPI: ModelAPIResponses, want: aipkg.APIOpenAICodexResponse},
		{provider: "openai", modelAPI: ModelAPIChatCompletions, want: aipkg.APIOpenAICompletions},
		{provider: "openrouter", modelAPI: ModelAPIResponses, want: aipkg.APIOpenAIResponses},
	}
	for _, tc := range cases {
		if got := derivePkgAIAPI(tc.provider, tc.modelAPI); got != tc.want {
			t.Fatalf("derivePkgAIAPI(%q,%q)=%q want=%q", tc.provider, tc.modelAPI, got, tc.want)
		}
	}
}

func TestDerivePkgAIModelDescriptor(t *testing.T) {
	model := derivePkgAIModelDescriptor(
		"anthropic/claude-sonnet-4-5",
		"claude-sonnet-4-5",
		"anthropic",
		ModelAPIResponses,
		64000,
	)
	if model.ID != "claude-sonnet-4-5" {
		t.Fatalf("expected api model id to be set, got %q", model.ID)
	}
	if model.Name != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("expected display model name from effectiveModel, got %q", model.Name)
	}
	if model.Provider != "anthropic" {
		t.Fatalf("expected provider anthropic, got %q", model.Provider)
	}
	if model.API != aipkg.APIAnthropicMessages {
		t.Fatalf("expected anthropic API mapping, got %q", model.API)
	}
	if model.MaxTokens != 64000 {
		t.Fatalf("expected max tokens propagated, got %d", model.MaxTokens)
	}
	if len(model.Input) != 1 || model.Input[0] != "text" {
		t.Fatalf("expected text-only input default, got %#v", model.Input)
	}
}
