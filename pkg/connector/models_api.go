package connector

// Model API provides a unified interface for looking up models and aliases.
// All data comes from the generated ModelManifest in models_generated.go.

// GetModelDisplayName returns a human-readable display name for a model.
// If the model is in the manifest, returns its configured name.
// Otherwise, returns the model ID as-is.
func GetModelDisplayName(modelID string) string {
	// Resolve any aliases first
	resolvedID := ResolveAlias(modelID)

	// Check manifest for display name
	if info, ok := ModelManifest.Models[resolvedID]; ok && info.Name != "" {
		return info.Name
	}

	// Fallback to the model ID
	return resolvedID
}

// GetOpenAIModels returns all models from the manifest that use the "openai" provider.
// These are models accessed directly via OpenAI's API.
func GetOpenAIModels() []ModelInfo {
	var models []ModelInfo
	for _, info := range ModelManifest.Models {
		if info.Provider == "openai" {
			models = append(models, info)
		}
	}
	return models
}

// GetOpenRouterModels returns all models from the manifest that use the "openrouter" provider.
// These are models accessed via OpenRouter's API (including Beeper's OpenRouter proxy).
func GetOpenRouterModels() []ModelInfo {
	var models []ModelInfo
	for _, info := range ModelManifest.Models {
		if info.Provider == "openrouter" {
			models = append(models, info)
		}
	}
	return models
}

// ResolveAlias returns the actual model ID for a given alias.
// If the input is not an alias, it returns the input unchanged.
func ResolveAlias(modelID string) string {
	if resolved, ok := ModelManifest.Aliases[modelID]; ok {
		return resolved
	}
	return modelID
}

// Note: IsValidOpenRouterModel is in model_cache.go for runtime validation
// against the live OpenRouter API. The manifest provides build-time definitions.
