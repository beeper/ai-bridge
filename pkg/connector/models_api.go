package connector

import "strings"

// OpenClawAliases provides OpenClaw-compatible shorthands and model ID aliases.
// These resolve to canonical model IDs in the local manifest.
var OpenClawAliases = map[string]string{
	// OpenClaw built-in shorthands
	"opus":         "anthropic/claude-opus-4.5",
	"sonnet":       "anthropic/claude-sonnet-4.5",
	"haiku":        "anthropic/claude-haiku-4.5",
	"gpt":          "openai/gpt-5.2",
	"gpt-mini":     "openai/gpt-5-mini",
	"gemini":       "google/gemini-3-pro-preview",
	"gemini-flash": "google/gemini-3-flash-preview",

	// OpenClaw model ID variants
	"anthropic/claude-opus-4-5":   "anthropic/claude-opus-4.5",
	"anthropic/claude-sonnet-4-5": "anthropic/claude-sonnet-4.5",
	"anthropic/claude-haiku-4-5":  "anthropic/claude-haiku-4.5",
	"zai/glm-4.7":                 "z-ai/glm-4.7",

	// OpenClaw provider IDs that differ from OpenRouter IDs
	"minimax/MiniMax-M2.1":          "minimax/minimax-m2.1",
	"minimax/MiniMax-M2":            "minimax/minimax-m2",
	"moonshot/kimi-k2.5":            "moonshotai/kimi-k2.5",
	"moonshot/kimi-k2-0905":         "moonshotai/kimi-k2-0905",
	"moonshot/kimi-k2-0905-preview": "moonshotai/kimi-k2-0905",
	"moonshot/kimi-k2-thinking":     "moonshotai/kimi-k2-thinking",
}

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

// GetOpenAIModels returns all models from the manifest with the "openai/" prefix.
// These models work with both OpenRouter and direct OpenAI API access.
func GetOpenAIModels() []ModelInfo {
	var models []ModelInfo
	for _, info := range ModelManifest.Models {
		if strings.HasPrefix(info.ID, "openai/") {
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
	normalized := strings.TrimSpace(modelID)
	if normalized == "" {
		return normalized
	}
	if resolved, ok := OpenClawAliases[normalized]; ok {
		return resolved
	}
	lower := strings.ToLower(normalized)
	if resolved, ok := OpenClawAliases[lower]; ok {
		return resolved
	}
	if resolved, ok := ModelManifest.Aliases[normalized]; ok {
		return resolved
	}
	if resolved, ok := ModelManifest.Aliases[lower]; ok {
		return resolved
	}
	return normalized
}

// Note: IsValidOpenRouterModel is in model_cache.go for runtime validation
// against the live OpenRouter API. The manifest provides build-time definitions.
