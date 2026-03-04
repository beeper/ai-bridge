package providers

import "github.com/beeper/ai-bridge/pkg/ai"

const BuiltinProviderSourceID = "pkg/ai/providers/register_builtins"

// RegisterBuiltInAPIProviders registers providers implemented in this package.
func RegisterBuiltInAPIProviders() {
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIOpenAIResponses,
		Stream:       streamOpenAIResponses,
		StreamSimple: streamSimpleOpenAIResponses,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIOpenAICompletions,
		Stream:       streamOpenAICompletions,
		StreamSimple: streamSimpleOpenAICompletions,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIAzureOpenAIResponse,
		Stream:       streamAzureOpenAIResponses,
		StreamSimple: streamSimpleAzureOpenAIResponses,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIOpenAICodexResponse,
		Stream:       streamOpenAICodexResponses,
		StreamSimple: streamSimpleOpenAICodexResponses,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIAnthropicMessages,
		Stream:       streamAnthropicMessages,
		StreamSimple: streamSimpleAnthropicMessages,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIGoogleGenerativeAI,
		Stream:       streamGoogleGenerativeAI,
		StreamSimple: streamSimpleGoogleGenerativeAI,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIGoogleVertex,
		Stream:       streamGoogleVertex,
		StreamSimple: streamSimpleGoogleVertex,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIBedrockConverse,
		Stream:       streamBedrockConverse,
		StreamSimple: streamSimpleBedrockConverse,
	}, BuiltinProviderSourceID)
	ai.RegisterAPIProvider(ai.APIProvider{
		API:          ai.APIGoogleGeminiCLI,
		Stream:       streamGoogleGeminiCLI,
		StreamSimple: streamSimpleGoogleGeminiCLI,
	}, BuiltinProviderSourceID)
}

func ResetAPIProviders() {
	ai.ClearAPIProviders()
	RegisterBuiltInAPIProviders()
}
