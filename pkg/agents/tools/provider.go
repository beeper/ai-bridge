package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Provider tools are markers for tools handled by the AI provider's API.
// They have no local Execute function - the provider handles execution.

// CodeInterpreter is the provider-native code execution tool.
// Used by providers like OpenAI that have built-in code interpreters.
var CodeInterpreter = &Tool{
	Tool: mcp.Tool{
		Name:        "code_interpreter",
		Description: "Execute Python code for calculations, data analysis, and file processing.",
		Annotations: &mcp.ToolAnnotations{Title: "Code Interpreter"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{
					"type":        "string",
					"description": "The Python code to execute",
				},
			},
			"required": []string{"code"},
		},
	},
	Type:    ToolTypeProvider,
	Group:   GroupCode,
	Execute: nil, // Handled by provider API
}

// IsProviderTool returns true if the tool is handled by the provider API.
func IsProviderTool(t *Tool) bool {
	return t.Type == ToolTypeProvider || t.Type == ToolTypePlugin
}

// ProviderTools returns all provider/plugin tools.
func ProviderTools() []*Tool {
	return []*Tool{
		CodeInterpreter,
	}
}
