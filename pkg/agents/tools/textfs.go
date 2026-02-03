package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

func execUnavailable(name string) func(ctx context.Context, input map[string]any) (*Result, error) {
	return func(ctx context.Context, input map[string]any) (*Result, error) {
		return ErrorResult(name, "tool execution is handled by the connector runtime"), nil
	}
}

var (
	ReadTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.ReadName,
			Description: toolspec.ReadDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Read"},
			InputSchema: toolspec.ReadSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.ReadName),
	}
	WriteTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.WriteName,
			Description: toolspec.WriteDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Write"},
			InputSchema: toolspec.WriteSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.WriteName),
	}
	EditTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.EditName,
			Description: toolspec.EditDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Edit"},
			InputSchema: toolspec.EditSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.EditName),
	}
	LsTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.LsName,
			Description: toolspec.LsDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Ls"},
			InputSchema: toolspec.LsSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.LsName),
	}
	FindTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.FindName,
			Description: toolspec.FindDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Find"},
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
			Annotations: &mcp.ToolAnnotations{Title: "Grep"},
			InputSchema: toolspec.GrepSchema(),
		},
		Type:    ToolTypeBuiltin,
		Group:   GroupFS,
		Execute: execUnavailable(toolspec.GrepName),
	}
)

