package connector

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	agenttools "github.com/beeper/ai-bridge/pkg/agents/tools"
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

func defaultWebSearchOpenRouterTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.WebSearchOpenRouterName,
		Description: toolspec.WebSearchOpenRouterDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Web Search (OpenRouter)"},
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

func defaultReadTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.ReadName,
		Description: toolspec.ReadDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Read"},
		InputSchema: toolspec.ReadSchema(),
	}
}

func defaultWriteTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.WriteName,
		Description: toolspec.WriteDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Write"},
		InputSchema: toolspec.WriteSchema(),
	}
}

func defaultEditTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.EditName,
		Description: toolspec.EditDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Edit"},
		InputSchema: toolspec.EditSchema(),
	}
}

func defaultLsTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.LsName,
		Description: toolspec.LsDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Ls"},
		InputSchema: toolspec.LsSchema(),
	}
}

func defaultFindTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.FindName,
		Description: toolspec.FindDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Find"},
		InputSchema: toolspec.FindSchema(),
	}
}

func defaultGrepTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.GrepName,
		Description: toolspec.GrepDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Grep"},
		InputSchema: toolspec.GrepSchema(),
	}
}

func defaultGravatarFetchTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.GravatarFetchName,
		Description: toolspec.GravatarFetchDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Gravatar Fetch"},
		InputSchema: toolspec.GravatarFetchSchema(),
	}
}

func defaultGravatarSetTool() mcp.Tool {
	return mcp.Tool{
		Name:        toolspec.GravatarSetName,
		Description: toolspec.GravatarSetDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Gravatar Set"},
		InputSchema: toolspec.GravatarSetSchema(),
	}
}

func defaultSessionsListTool() mcp.Tool {
	return agenttools.SessionsListTool.Tool
}

func defaultSessionsHistoryTool() mcp.Tool {
	return agenttools.SessionsHistoryTool.Tool
}

func defaultSessionsSendTool() mcp.Tool {
	return agenttools.SessionsSendTool.Tool
}
