package connector

import (
	"strings"

	aipkg "github.com/beeper/ai-bridge/pkg/ai"
)

func derivePkgAIAPI(provider string, modelAPI ModelAPI) aipkg.Api {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		return aipkg.APIAnthropicMessages
	case "google":
		return aipkg.APIGoogleGenerativeAI
	case "google-vertex":
		return aipkg.APIGoogleVertex
	case "google-gemini-cli", "google-antigravity":
		return aipkg.APIGoogleGeminiCLI
	case "azure-openai-responses":
		return aipkg.APIAzureOpenAIResponse
	case "amazon-bedrock":
		return aipkg.APIBedrockConverse
	case "openai-codex":
		return aipkg.APIOpenAICodexResponse
	}

	if modelAPI == ModelAPIChatCompletions {
		return aipkg.APIOpenAICompletions
	}
	return aipkg.APIOpenAIResponses
}

func derivePkgAIModelDescriptor(
	effectiveModel string,
	effectiveModelForAPI string,
	provider string,
	modelAPI ModelAPI,
	maxTokens int,
) aipkg.Model {
	name := strings.TrimSpace(effectiveModel)
	if name == "" {
		name = strings.TrimSpace(effectiveModelForAPI)
	}
	return aipkg.Model{
		ID:        strings.TrimSpace(effectiveModelForAPI),
		Name:      name,
		Provider:  aipkg.Provider(strings.TrimSpace(provider)),
		API:       derivePkgAIAPI(provider, modelAPI),
		Input:     []string{"text"},
		MaxTokens: maxTokens,
	}
}
