package runtime

import (
	"encoding/json"

	"github.com/beeper/ai-bridge/pkg/core/shared/toolspec"
)

func buildBuiltinToolDefinitions() []ToolDefinition {
	// Simple bridge intentionally exposes only web_search as a callable tool.
	return []ToolDefinition{
		{
			Name:        toolspec.WebSearchName,
			Description: toolspec.WebSearchDescription,
			Parameters:  toolspec.WebSearchSchema(),
			Execute:     executeWebSearch,
		},
	}
}

func toolSchemaToMap(schema any) map[string]any {
	switch v := schema.(type) {
	case nil:
		return nil
	case map[string]any:
		return v
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(encoded, &out); err != nil {
			return nil
		}
		return out
	}
}
