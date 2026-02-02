package connector

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

func defaultCalculatorTool() mcp.Tool {
	return mcp.Tool{
		Name:        ToolNameCalculator,
		Description: toolspec.CalculatorDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Calculator"},
	}
}

func defaultWebSearchTool() mcp.Tool {
	return mcp.Tool{
		Name:        ToolNameWebSearch,
		Description: toolspec.WebSearchDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Web Search"},
	}
}

func defaultChatInfoTool() mcp.Tool {
	return mcp.Tool{
		Name:        ToolNameSetChatInfo,
		Description: toolspec.SetChatInfoDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Set Chat Info"},
	}
}

func defaultMessageTool() mcp.Tool {
	return mcp.Tool{
		Name:        ToolNameMessage,
		Description: toolspec.MessageDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Message"},
	}
}
