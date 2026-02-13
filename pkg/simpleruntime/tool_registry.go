//lint:file-ignore U1000 Hard-cut compatibility: pending full dead-code deletion.
package connector

import (
	"context"
	"encoding/json"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

type toolExecutor func(ctx context.Context, args map[string]any) (string, error)

func builtinToolExecutors() map[string]toolExecutor {
	return map[string]toolExecutor{
		toolspec.WebSearchName: executeWebSearch,
	}
}

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
