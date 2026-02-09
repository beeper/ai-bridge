package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

// BeeperDocsTool is the Beeper help documentation search tool.
var BeeperDocsTool = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.BeeperDocsName,
		Description: toolspec.BeeperDocsDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Beeper Docs"},
		InputSchema: toolspec.BeeperDocsSchema(),
	},
	Type:    ToolTypeBuiltin,
	Group:   GroupWeb,
	Execute: executeBeeperDocsPlaceholder,
}

// executeBeeperDocsPlaceholder is a no-op; real execution happens in the connector.
func executeBeeperDocsPlaceholder(_ context.Context, _ map[string]any) (*Result, error) {
	return ErrorResult("beeper_docs", "beeper_docs is only available through the connector"), nil
}
