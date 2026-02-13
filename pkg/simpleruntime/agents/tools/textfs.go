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
)
