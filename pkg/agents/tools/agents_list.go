package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// AgentsListTool lists agent ids allowed for sessions_spawn.
var AgentsListTool = &Tool{
	Tool: mcp.Tool{
		Name:        "agents_list",
		Description: "List agent ids you can target with sessions_spawn (based on allowlists).",
		Annotations: &mcp.ToolAnnotations{Title: "Agents List"},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	Type:  ToolTypeBuiltin,
	Group: GroupSessions,
}
