package connector

// Model API provides a unified interface for looking up models and aliases.
// All data comes from the generated ModelManifest in models_generated.go.

// GetModel returns the ModelInfo for a given model ID.
// It first resolves any aliases, then looks up the model in the manifest.
// Returns nil if the model is not found.
func GetModel(modelID string) *ModelInfo {
	// Resolve any aliases first
	resolvedID := ResolveAlias(modelID)

	// Look up in manifest
	if info, ok := ModelManifest.Models[resolvedID]; ok {
		return &info
	}
	return nil
}

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

// IsKnownModel checks if a model ID (or alias) is in the manifest.
func IsKnownModel(modelID string) bool {
	// Check if it's an alias
	if _, ok := ModelManifest.Aliases[modelID]; ok {
		return true
	}
	// Check if it's a direct model ID
	_, ok := ModelManifest.Models[modelID]
	return ok
}

// GetBeeperModels returns all models from the manifest that use the "openrouter" provider.
// This is the primary list of models available in Beeper mode.
func GetBeeperModels() []ModelInfo {
	var models []ModelInfo
	for _, info := range ModelManifest.Models {
		if info.Provider == "openrouter" {
			models = append(models, info)
		}
	}
	return models
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

// ResolveAlias returns the actual model ID for a given alias.
// If the input is not an alias, it returns the input unchanged.
func ResolveAlias(modelID string) string {
	if resolved, ok := ModelManifest.Aliases[modelID]; ok {
		return resolved
	}
	return modelID
}

// GetAliases returns a copy of all defined model aliases.
func GetAliases() map[string]string {
	result := make(map[string]string, len(ModelManifest.Aliases))
	for k, v := range ModelManifest.Aliases {
		result[k] = v
	}
	return result
}

// Note: IsValidOpenRouterModel is in model_cache.go for runtime validation
// against the live OpenRouter API. The manifest provides build-time definitions.
