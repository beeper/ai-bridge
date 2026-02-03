package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

var (
	LSTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.LSName,
			Description: toolspec.LSDescription,
			Annotations: &mcp.ToolAnnotations{Title: "ls"},
			InputSchema: toolspec.LSSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.LSName),
	}
	FindTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.FindName,
			Description: toolspec.FindDescription,
			Annotations: &mcp.ToolAnnotations{Title: "find"},
			InputSchema: toolspec.FindSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.FindName),
	}
	GrepTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.GrepName,
			Description: toolspec.GrepDescription,
			Annotations: &mcp.ToolAnnotations{Title: "grep"},
			InputSchema: toolspec.GrepSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.GrepName),
	}
)
