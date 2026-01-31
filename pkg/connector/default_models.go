package connector

import "slices"

// DefaultModels contains all default model definitions for each provider.
var DefaultModels = map[string][]ModelInfo{
	"openai": {
		{
			ID:                  "openai/o1",
			Name:                "O1",
			Provider:            "openai",
			Description:         "Advanced reasoning model",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   true,
			SupportsWebSearch:   true,
			ContextWindow:       200000,
			MaxOutputTokens:     100000,
			AvailableTools:      []string{ToolWebSearch, ToolFunctionCalling},
		},
		{
			ID:                  "openai/o1-mini",
			Name:                "O1 Mini",
			Provider:            "openai",
			Description:         "Fast reasoning model",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   true,
			SupportsWebSearch:   true,
			ContextWindow:       128000,
			MaxOutputTokens:     65536,
			AvailableTools:      []string{ToolWebSearch, ToolFunctionCalling},
		},
		{
			ID:                  "openai/o3-mini",
			Name:                "O3 Mini",
			Provider:            "openai",
			Description:         "Latest reasoning model",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   true,
			SupportsWebSearch:   true,
			ContextWindow:       200000,
			MaxOutputTokens:     100000,
			AvailableTools:      []string{ToolWebSearch, ToolFunctionCalling},
		},
		{
			ID:                  "openai/gpt-4-turbo",
			Name:                "GPT 4 Turbo",
			Provider:            "openai",
			Description:         "Previous generation GPT-4 with vision",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   false,
			SupportsWebSearch:   true,
			ContextWindow:       128000,
			MaxOutputTokens:     4096,
			AvailableTools:      []string{ToolWebSearch, ToolFunctionCalling},
		},
	},
}

// GetDefaultModels returns default models for a provider.
// Returns a copy to prevent modification of the original slice.
func GetDefaultModels(provider string) []ModelInfo {
	models, ok := DefaultModels[provider]
	if !ok {
		return nil
	}
	return slices.Clone(models)
}

// GetAllBeeperModels returns all Beeper models from the generated file.
// This is the source of truth for Beeper model capabilities.
func GetAllBeeperModels() []ModelInfo {
	return GetBeeperModelsGenerated()
}
