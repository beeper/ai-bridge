package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

// Provider tools are markers for tools handled by the AI provider's API.
// They have no local Execute function - the provider handles execution.

var (
	MessageTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.MessageName,
			Description: toolspec.MessageDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Message"},
			InputSchema: toolspec.MessageSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMessaging,
	}
	WebFetchTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.WebFetchName,
			Description: toolspec.WebFetchDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Web Fetch"},
			InputSchema: toolspec.WebFetchSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupWeb,
	}
	SessionStatusTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.SessionStatusName,
			Description: toolspec.SessionStatusDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Session Status"},
			InputSchema: toolspec.SessionStatusSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupStatus,
	}
	MemorySearchTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.MemorySearchName,
			Description: toolspec.MemorySearchDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Memory Search"},
			InputSchema: toolspec.MemorySearchSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMemory,
	}
	MemoryGetTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.MemoryGetName,
			Description: toolspec.MemoryGetDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Memory Get"},
			InputSchema: toolspec.MemoryGetSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMemory,
	}
	MemoryStoreTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.MemoryStoreName,
			Description: toolspec.MemoryStoreDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Memory Store"},
			InputSchema: toolspec.MemoryStoreSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMemory,
	}
	MemoryForgetTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.MemoryForgetName,
			Description: toolspec.MemoryForgetDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Memory Forget"},
			InputSchema: toolspec.MemoryForgetSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMemory,
	}
	ImageTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.ImageName,
			Description: toolspec.ImageDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Image"},
			InputSchema: toolspec.ImageSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMedia,
	}
	TTSTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.TTSName,
			Description: toolspec.TTSDescription,
			Annotations: &mcp.ToolAnnotations{Title: "TTS"},
			InputSchema: toolspec.TTSSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMedia,
	}
	AnalyzeImageTool = &Tool{
		Tool: mcp.Tool{
			Name:        toolspec.AnalyzeImageName,
			Description: toolspec.AnalyzeImageDescription,
			Annotations: &mcp.ToolAnnotations{Title: "Analyze Image"},
			InputSchema: toolspec.AnalyzeImageSchema(),
		},
		Type:  ToolTypeProvider,
		Group: GroupMedia,
	}
)

// IsProviderTool returns true if the tool is handled by the provider API.
func IsProviderTool(t *Tool) bool {
	return t.Type == ToolTypeProvider || t.Type == ToolTypePlugin
}

// ProviderTools returns all provider/plugin tools.
func ProviderTools() []*Tool {
	return []*Tool{
		MessageTool,
		WebFetchTool,
		SessionStatusTool,
		MemorySearchTool,
		MemoryGetTool,
		MemoryStoreTool,
		MemoryForgetTool,
		ImageTool,
		TTSTool,
		AnalyzeImageTool,
	}
}
