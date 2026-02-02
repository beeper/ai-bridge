package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

// ChatInfo patches the chat title and/or description.
var ChatInfo = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.SetChatInfoName,
		Description: toolspec.SetChatInfoDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Set Chat Info"},
		InputSchema: toolspec.SetChatInfoSchema(),
	},
	Type:  ToolTypeBuiltin,
	Group: GroupChat,
	Execute: func(_ context.Context, _ map[string]any) (*Result, error) {
		return ErrorResult("set_chat_info", "set_chat_info is handled by the bridge"), nil
	},
}
