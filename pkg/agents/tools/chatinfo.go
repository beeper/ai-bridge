package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ChatInfo patches the chat title and/or description.
var ChatInfo = &Tool{
	Tool: mcp.Tool{
		Name:        "set_chat_info",
		Description: "Patch the chat title and/or description (omit fields to keep them unchanged).",
		Annotations: &mcp.ToolAnnotations{Title: "Set Chat Info"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Optional. The new title for the chat",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Optional. The new description/topic for the chat (empty string clears it)",
				},
			},
			"minProperties":        1,
			"additionalProperties": false,
		},
	},
	Type:  ToolTypeBuiltin,
	Group: GroupChat,
	Execute: func(_ context.Context, _ map[string]any) (*Result, error) {
		return ErrorResult("set_chat_info", "set_chat_info is handled by the bridge"), nil
	},
}
