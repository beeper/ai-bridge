package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

// BeeperSendFeedbackTool is the Beeper feedback submission tool.
var BeeperSendFeedbackTool = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.BeeperSendFeedbackName,
		Description: toolspec.BeeperSendFeedbackDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Beeper Send Feedback"},
		InputSchema: toolspec.BeeperSendFeedbackSchema(),
	},
	Type:    ToolTypeBuiltin,
	Group:   GroupWeb,
	Execute: executeBeeperSendFeedbackPlaceholder,
}

// executeBeeperSendFeedbackPlaceholder is a no-op; real execution happens in the connector.
func executeBeeperSendFeedbackPlaceholder(_ context.Context, _ map[string]any) (*Result, error) {
	return ErrorResult("beeper_send_feedback", "beeper_send_feedback is only available through the connector"), nil
}
