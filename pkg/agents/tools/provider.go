package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Provider tools are markers for tools handled by the AI provider's API.
// They have no local Execute function - the provider handles execution.

// WebSearchProvider is the provider-native web search tool.
// Used by providers like OpenAI, Anthropic that have built-in search.
var WebSearchProvider = &Tool{
	Tool: mcp.Tool{
		Name:        "web_search_provider",
		Description: "Search the web using the AI provider's built-in search capability.",
		Annotations: &mcp.ToolAnnotations{Title: "Provider Web Search"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
				},
			},
			"required": []string{"query"},
		},
	},
	Type:    ToolTypeProvider,
	Group:   GroupSearch,
	Execute: nil, // Handled by provider API
}

// CodeInterpreter is the provider-native code execution tool.
// Used by providers like OpenAI that have built-in code interpreters.
var CodeInterpreter = &Tool{
	Tool: mcp.Tool{
		Name:        "code_interpreter",
		Description: "Execute code using the AI provider's built-in code interpreter.",
		Annotations: &mcp.ToolAnnotations{Title: "Code Interpreter"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{
					"type":        "string",
					"description": "The code to execute",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "The programming language (e.g., python)",
					"default":     "python",
				},
			},
			"required": []string{"code"},
		},
	},
	Type:    ToolTypeProvider,
	Group:   GroupCode,
	Execute: nil, // Handled by provider API
}

// OnlinePlugin is the OpenRouter :online plugin for real-time web search.
var OnlinePlugin = &Tool{
	Tool: mcp.Tool{
		Name:        ":online",
		Description: "Real-time web search using OpenRouter's online plugin.",
		Annotations: &mcp.ToolAnnotations{Title: "Online Search"},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	Type:    ToolTypePlugin,
	Group:   GroupOnline,
	Execute: nil, // Handled by OpenRouter
}

// IsProviderTool returns true if the tool is handled by the provider API.
func IsProviderTool(t *Tool) bool {
	return t.Type == ToolTypeProvider || t.Type == ToolTypePlugin
}

// ProviderTools returns all provider/plugin tools.
func ProviderTools() []*Tool {
	return []*Tool{
		WebSearchProvider,
		CodeInterpreter,
		OnlinePlugin,
	}
}
