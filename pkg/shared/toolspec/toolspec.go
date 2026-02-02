package toolspec

// Shared tool schema definitions used by both connector and agents.

const (
	CalculatorName        = "calculator"
	CalculatorDescription = "Perform basic arithmetic calculations. Supports addition, subtraction, multiplication, division, and modulo operations."

	WebSearchName        = "web_search"
	WebSearchDescription = "Search the web for information. Returns a summary of search results."

	WebFetchName        = "web_fetch"
	WebFetchDescription = "Fetch a web page and extract its readable content as text or markdown."

	SetChatInfoName        = "set_chat_info"
	SetChatInfoDescription = "Patch the chat title and/or description (omit fields to keep them unchanged)."

	MessageName        = "message"
	MessageDescription = "Send messages and perform channel actions in the current chat. Supports: send, react, reactions, edit, delete, reply, pin, unpin, list-pins, thread-reply, search, read, member-info, channel-info."

	SessionStatusName        = "session_status"
	SessionStatusDescription = "Get current session status including time, date, model info, and context usage. Use this tool when asked about current time, date, day of week, or what model is being used."

	ImageName        = "image"
	ImageDescription = "Generate an image from a text prompt using AI image generation."

	TTSName        = "tts"
	TTSDescription = "Convert text to speech audio. Returns audio that will be sent as a voice message. Only available on Beeper and OpenAI providers."

	AnalyzeImageName        = "analyze_image"
	AnalyzeImageDescription = "Analyze an image with a custom prompt. Use this to examine image details, read text from images (OCR), identify objects, or get specific information about visual content."

	MemorySearchName        = "memory_search"
	MemorySearchDescription = "Search your memory for relevant information. Use this to recall facts, preferences, decisions, or context from previous conversations."
	MemoryGetName           = "memory_get"
	MemoryGetDescription    = "Retrieve the full content of a specific memory by its path."
	MemoryStoreName         = "memory_store"
	MemoryStoreDescription  = "Store a new memory for later recall. Use this to remember important facts, user preferences, decisions, or context that should persist across conversations."
	MemoryForgetName        = "memory_forget"
	MemoryForgetDescription = "Remove a memory by its ID/path. Use this to delete outdated or incorrect information."
)

// CalculatorSchema returns the JSON schema for the calculator tool.
func CalculatorSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{
				"type":        "string",
				"description": "A mathematical expression to evaluate, e.g. '2 + 3 * 4' or '100 / 5'",
			},
		},
		"required": []string{"expression"},
	}
}

// WebSearchSchema returns the JSON schema for the web search tool.
func WebSearchSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query",
			},
		},
		"required": []string{"query"},
	}
}

// WebFetchSchema returns the JSON schema for the web fetch tool.
func WebFetchSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch (must be http or https)",
			},
			"max_chars": map[string]any{
				"type":        "number",
				"description": "Maximum characters to return (default: 50000)",
			},
		},
		"required": []string{"url"},
	}
}

// SetChatInfoSchema returns the JSON schema for the set_chat_info tool.
func SetChatInfoSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Optional. The new title for the chat",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional. The new description/topic for the chat (empty string clears it)",
			},
		},
		"minProperties":        1,
		"additionalProperties": false,
	}
}

// MessageSchema returns the JSON schema for the message tool.
func MessageSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"send", "react", "reactions", "edit", "delete", "reply", "pin", "unpin", "list-pins", "thread-reply", "search", "read", "member-info", "channel-info"},
				"description": "The action to perform",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "For send/edit/reply/thread-reply: the message text",
			},
			"message_id": map[string]any{
				"type":        "string",
				"description": "Target message ID for react/reactions/edit/delete/reply/pin/unpin/thread-reply/read",
			},
			"emoji": map[string]any{
				"type":        "string",
				"description": "For action=react: the emoji to react with (empty to remove all reactions)",
			},
			"remove": map[string]any{
				"type":        "boolean",
				"description": "For action=react: set true to remove the reaction instead of adding",
			},
			"user_id": map[string]any{
				"type":        "string",
				"description": "For action=member-info: the Matrix user ID to look up (e.g., @user:server.com)",
			},
			"thread_id": map[string]any{
				"type":        "string",
				"description": "For action=thread-reply: the thread root message ID",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "For action=search: search query to find messages",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "For action=search: max results to return (default: 20)",
			},
		},
		"required": []string{"action"},
	}
}

// SessionStatusSchema returns the JSON schema for the session_status tool.
func SessionStatusSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"set_model": map[string]any{
				"type":        "string",
				"description": "Optional: change the model for this session (e.g., 'gpt-4o', 'claude-sonnet-4-20250514')",
			},
		},
	}
}

// ImageSchema returns the JSON schema for the image tool.
func ImageSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The text prompt describing the image to generate",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Image model to use (default: google/gemini-3-pro-image-preview)",
			},
		},
		"required": []string{"prompt"},
	}
}

// TTSSchema returns the JSON schema for the tts tool.
func TTSSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "The text to convert to speech (max 4096 characters)",
			},
			"voice": map[string]any{
				"type":        "string",
				"enum":        []string{"alloy", "ash", "coral", "echo", "fable", "onyx", "nova", "sage", "shimmer"},
				"description": "The voice to use for speech synthesis (default: alloy)",
			},
		},
		"required": []string{"text"},
	}
}

// AnalyzeImageSchema returns the JSON schema for the analyze_image tool.
func AnalyzeImageSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"image_url": map[string]any{
				"type":        "string",
				"description": "URL of the image to analyze (http/https URL, mxc:// Matrix URL, or data: URI with base64)",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "What to analyze or look for in the image (e.g., 'describe this image', 'read the text', 'what objects are visible')",
			},
		},
		"required": []string{"image_url", "prompt"},
	}
}

// MemorySearchSchema returns the JSON schema for the memory_search tool.
func MemorySearchSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query to find relevant memories",
			},
			"maxResults": map[string]any{
				"type":        "number",
				"description": "Maximum number of results to return (default: 6)",
			},
			"minScore": map[string]any{
				"type":        "number",
				"description": "Minimum relevance score threshold (0-1, default: 0.35)",
			},
		},
		"required": []string{"query"},
	}
}

// MemoryGetSchema returns the JSON schema for the memory_get tool.
func MemoryGetSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The memory path (e.g., 'agent:myagent/fact:abc123' or 'global/fact:xyz789')",
			},
			"from": map[string]any{
				"type":        "number",
				"description": "Optional: starting line (ignored for Matrix)",
			},
			"lines": map[string]any{
				"type":        "number",
				"description": "Optional: number of lines (ignored for Matrix)",
			},
		},
		"required": []string{"path"},
	}
}

// MemoryStoreSchema returns the JSON schema for the memory_store tool.
func MemoryStoreSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The content to store in memory",
			},
			"importance": map[string]any{
				"type":        "number",
				"description": "Importance score from 0 to 1 (default: 0.5). Higher values make the memory more likely to surface in searches.",
			},
			"category": map[string]any{
				"type":        "string",
				"enum":        []string{"preference", "decision", "entity", "fact", "other"},
				"description": "Category of memory (default: 'other')",
			},
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{"agent", "global"},
				"description": "Where to store the memory: 'agent' for this agent only, 'global' for all agents (default: 'agent')",
			},
		},
		"required": []string{"content"},
	}
}

// MemoryForgetSchema returns the JSON schema for the memory_forget tool.
func MemoryForgetSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The memory ID or path to forget",
			},
		},
		"required": []string{"id"},
	}
}
