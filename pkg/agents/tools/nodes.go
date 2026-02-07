package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

// NodesTool is an OpenClaw-compatible "nodes" tool (backed by OpenClaw gateway when configured).
var NodesTool = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.NodesName,
		Description: toolspec.NodesDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Nodes"},
		InputSchema: toolspec.NodesSchema(),
	},
	Type:  ToolTypeBuiltin,
	Group: GroupNodes,
}

