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
		InputSchema: toolspec.CalculatorSchema(),
	}
}

func defaultWebSearchTool() mcp.Tool {
	return mcp.Tool{
		Name:        ToolNameWebSearch,
		Description: toolspec.WebSearchDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Web Search"},
		InputSchema: toolspec.WebSearchSchema(),
	}
}

func defaultWebFetchTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.WebFetchName,
		Description: toolspec.WebFetchDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Web Fetch"},
		InputSchema: toolspec.WebFetchSchema(),
	}
}

func defaultChatInfoTool() mcp.Tool {
	return mcp.Tool{
		Name:        ToolNameSetChatInfo,
		Description: toolspec.SetChatInfoDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Set Chat Info"},
		InputSchema: toolspec.SetChatInfoSchema(),
	}
}

func defaultMessageTool() mcp.Tool {
	return mcp.Tool{
		Name:        ToolNameMessage,
		Description: toolspec.MessageDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Message"},
		InputSchema: toolspec.MessageSchema(),
	}
}

func defaultSessionStatusTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.SessionStatusName,
		Description: toolspec.SessionStatusDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Session Status"},
		InputSchema: toolspec.SessionStatusSchema(),
	}
}

func defaultMemorySearchTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.MemorySearchName,
		Description: toolspec.MemorySearchDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Memory Search"},
		InputSchema: toolspec.MemorySearchSchema(),
	}
}

func defaultMemoryGetTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.MemoryGetName,
		Description: toolspec.MemoryGetDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Memory Get"},
		InputSchema: toolspec.MemoryGetSchema(),
	}
}

func defaultMemoryStoreTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.MemoryStoreName,
		Description: toolspec.MemoryStoreDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Memory Store"},
		InputSchema: toolspec.MemoryStoreSchema(),
	}
}

func defaultMemoryForgetTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.MemoryForgetName,
		Description: toolspec.MemoryForgetDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Memory Forget"},
		InputSchema: toolspec.MemoryForgetSchema(),
	}
}
