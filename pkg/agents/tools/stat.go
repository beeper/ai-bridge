package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

// StatTool provides metadata for a file or directory.
var StatTool = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.StatName,
		Description: toolspec.StatDescription,
		Annotations: &mcp.ToolAnnotations{Title: "stat"},
		InputSchema: toolspec.StatSchema(),
	},
	Type:    ToolTypeBuiltin,
	Group:   GroupFS,
	Execute: execUnavailable(toolspec.StatName),
}
