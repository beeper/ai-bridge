package connector

import "maps"

// ModelRoutes defines aliases from user-facing model names to actual OpenRouter models.
// This allows for stable model identifiers that can be remapped as new models are released.
var ModelRoutes = map[string]string{
	// Default alias
	"beeper/default": DefaultModelBeeper,

	// Stable aliases that can be remapped
	"beeper/fast":      "openai/gpt-5-mini",
	"beeper/smart":     "openai/gpt-5.2",
	"beeper/reasoning": "openai/gpt-5.2", // Uses reasoning effort parameter
}

// ResolveModelAlias returns the actual model ID for a given input.
// If the input is a known alias, it returns the mapped model.
// Otherwise, it returns the input unchanged.
func ResolveModelAlias(modelID string) string {
	if resolved, ok := ModelRoutes[modelID]; ok {
		return resolved
	}
	return modelID
}

// GetModelAliases returns all defined model aliases
func GetModelAliases() map[string]string {
	// Return a copy to prevent modification
	result := make(map[string]string, len(ModelRoutes))
	maps.Copy(result, ModelRoutes)
	return result
}

// LookupModelInfo returns the ModelInfo for a given model ID.
// It first resolves any aliases, then looks up the model in the Beeper models list.
func LookupModelInfo(modelID string) *ModelInfo {
	// Resolve any aliases first
	resolvedID := ResolveModelAlias(modelID)

	// Look up in Beeper models
	for _, model := range GetDefaultModels("beeper") {
		if model.ID == resolvedID {
			return &model
		}
	}

	return nil
}
