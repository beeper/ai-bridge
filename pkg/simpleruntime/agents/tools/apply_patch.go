package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

var ApplyPatchTool = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.ApplyPatchName,
		Description: toolspec.ApplyPatchDescription,
		Annotations: &mcp.ToolAnnotations{Title: "apply_patch"},
		InputSchema: toolspec.ApplyPatchSchema(),
	},
	Type:    ToolTypeBuiltin,
	Group:   GroupFS,
	Execute: execUnavailable(toolspec.ApplyPatchName),
}
