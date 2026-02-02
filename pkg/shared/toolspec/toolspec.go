package toolspec

// Shared tool schema definitions used by both connector and agents.

const (
	CalculatorName        = "calculator"
	CalculatorDescription = "Perform basic arithmetic calculations. Supports addition, subtraction, multiplication, division, and modulo operations."

	WebSearchName        = "web_search"
	WebSearchDescription = "Search the web for information. Returns a summary of search results."

	WebFetchName        = "web_fetch"
	WebFetchDescription = "Fetch a web page and extract its readable content as text or markdown."

	MessageName        = "message"
	MessageDescription = "Send messages and perform channel actions in the current chat. Supports: send, sendWithEffect, broadcast, react, reactions, edit, delete/unsend, reply, pin, unpin, list-pins, thread-reply, search, read, member-info, channel-info, channel-edit."

	SessionStatusName        = "session_status"
	SessionStatusDescription = "Get current session status including time, date, model info, and context usage. Use this tool when asked about current time, date, day of week, or what model is being used."

	// ImageName matches OpenClaw's image analysis tool (vision).
	ImageName        = "image"
	ImageDescription = "Analyze an image with a custom prompt. Use this to examine image details, read text from images (OCR), identify objects, or get specific information about visual content."

	// ImageGenerateName is an AI image generation tool (not in OpenClaw).
	ImageGenerateName        = "image_generate"
	ImageGenerateDescription = "Generate one or more images from a text prompt. Supports provider-specific controls such as size, quality, style, background, output format, resolution, and optional input images for editing/composition."

	TTSName        = "tts"
	TTSDescription = "Convert text to speech audio. Returns audio that will be sent as a voice message. Only available on Beeper and OpenAI providers."

	// AnalyzeImageName is a deprecated alias for ImageName.
	AnalyzeImageName        = "analyze_image"
	AnalyzeImageDescription = "Deprecated alias for image (vision analysis)."

	MemorySearchName        = "memory_search"
	MemorySearchDescription = "Search your memory for relevant information. Use this to recall facts, preferences, decisions, or context from previous conversations."
	MemoryGetName           = "memory_get"
	MemoryGetDescription    = "Retrieve the full content of a specific memory by its path."
	MemoryStoreName         = "memory_store"
	MemoryStoreDescription  = "Store a new memory for later recall. Use this to remember important facts, user preferences, decisions, or context that should persist across conversations."
	MemoryForgetName        = "memory_forget"
	MemoryForgetDescription = "Remove a memory by its ID/path. Use this to delete outdated or incorrect information."

	GravatarFetchName        = "gravatar_fetch"
	GravatarFetchDescription = "Fetch a Gravatar profile for an email address."
	GravatarSetName          = "gravatar_set"
	GravatarSetDescription   = "Set the primary Gravatar profile for this login."
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
			"count": map[string]any{
				"type":        "number",
				"description": "Optional: max results to return (1-10). OpenClaw uses count.",
				"minimum":     1,
				"maximum":     10,
			},
			"country": map[string]any{
				"type":        "string",
				"description": "Optional: 2-letter country code for region-specific results (e.g., 'US', 'DE', 'ALL').",
			},
			"search_lang": map[string]any{
				"type":        "string",
				"description": "Optional: ISO language code for search results (e.g., 'en', 'de').",
			},
			"ui_lang": map[string]any{
				"type":        "string",
				"description": "Optional: ISO language code for UI elements.",
			},
			"freshness": map[string]any{
				"type":        "string",
				"description": "Optional: time filter ('pd', 'pw', 'pm', 'py') or range 'YYYY-MM-DDtoYYYY-MM-DD'.",
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
			"maxChars": map[string]any{
				"type":        "number",
				"description": "OpenClaw-style alias for max_chars",
			},
			"extractMode": map[string]any{
				"type":        "string",
				"enum":        []string{"markdown", "text"},
				"description": "Preferred extraction mode (alias only; markdown default)",
			},
		},
		"required": []string{"url"},
	}
}

// GravatarFetchSchema returns the JSON schema for the Gravatar fetch tool.
func GravatarFetchSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email": map[string]any{
				"type":        "string",
				"description": "Email address to fetch from Gravatar. If omitted, uses the stored Gravatar email.",
			},
		},
	}
}

// GravatarSetSchema returns the JSON schema for the Gravatar set tool.
func GravatarSetSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email": map[string]any{
				"type":        "string",
				"description": "Email address to set as the primary Gravatar profile.",
			},
		},
		"required": []string{"email"},
	}
}

// MessageSchema returns the JSON schema for the message tool.
func MessageSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"send", "sendWithEffect", "broadcast", "react", "reactions", "edit", "delete", "unsend", "reply", "pin", "unpin", "list-pins", "thread-reply", "search", "read", "member-info", "channel-info", "channel-edit"},
				"description": "The action to perform",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "For send/edit/reply/thread-reply: the message text",
			},
			"effectId": map[string]any{
				"type":        "string",
				"description": "Optional: message effect name/id for sendWithEffect (ignored by bridge).",
			},
			"effect": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style alias for effectId (ignored by bridge).",
			},
			"media": map[string]any{
				"type":        "string",
				"description": "Optional: media URL/path/data URL to send (image/audio/video/file).",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "Optional: filename for media uploads.",
			},
			"buffer": map[string]any{
				"type":        "string",
				"description": "Optional: base64 payload for attachments (optionally a data: URL).",
			},
			"contentType": map[string]any{
				"type":        "string",
				"description": "Optional: content type override for attachments (alias for mimeType).",
			},
			"mimeType": map[string]any{
				"type":        "string",
				"description": "Optional: content type override for attachments.",
			},
			"caption": map[string]any{
				"type":        "string",
				"description": "Optional: caption for media uploads.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional: file path to upload (alias for media).",
			},
			"filePath": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style alias for path.",
			},
			"message_id": map[string]any{
				"type":        "string",
				"description": "Target message ID for react/reactions/edit/delete/reply/pin/unpin/thread-reply/read",
			},
			"messageId": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style alias for message_id",
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
			"threadId": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style alias for thread_id",
			},
			"replyTo": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style alias for message_id when replying",
			},
			"asVoice": map[string]any{
				"type":        "boolean",
				"description": "Optional: send audio as a voice message (when media is audio).",
			},
			"silent": map[string]any{
				"type":        "boolean",
				"description": "Optional: send silently (ignored by bridge).",
			},
			"quoteText": map[string]any{
				"type":        "string",
				"description": "Optional: quote text for replies (ignored by bridge).",
			},
			"bestEffort": map[string]any{
				"type":        "boolean",
				"description": "Optional: best-effort delivery flag (ignored by bridge).",
			},
			"gifPlayback": map[string]any{
				"type":        "boolean",
				"description": "Optional: treat video media as GIF playback (sets MauGIF flag).",
			},
			"buttons": map[string]any{
				"type":        "array",
				"description": "Optional: inline keyboard buttons (ignored by bridge).",
			},
			"card": map[string]any{
				"type":        "object",
				"description": "Optional: adaptive card payload (ignored by bridge).",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "For action=search: search query to find messages",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "For action=search: max results to return (default: 20)",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "For action=channel-edit: new channel/room name",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "For action=channel-edit: alias for name",
			},
			"topic": map[string]any{
				"type":        "string",
				"description": "For action=channel-edit: new channel/room topic",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "For action=channel-edit: alias for topic",
			},
			"channel": map[string]any{
				"type":        "string",
				"description": "Optional: channel override (ignored by bridge; current room only).",
			},
			"target": map[string]any{
				"type":        "string",
				"description": "Optional: target override (ignored by bridge; current room only).",
			},
			"targets": map[string]any{
				"type":        "array",
				"description": "Optional: multi-target override (ignored by bridge; current room only).",
			},
			"accountId": map[string]any{
				"type":        "string",
				"description": "Optional: account override (ignored by bridge).",
			},
			"dryRun": map[string]any{
				"type":        "boolean",
				"description": "Optional: dry run (ignored by bridge).",
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
			"model": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style alias for set_model (use 'default' to reset override)",
			},
			"sessionKey": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style session key (ignored; current room only)",
			},
		},
	}
}

// ImageSchema returns the JSON schema for the OpenClaw image (vision) tool.
func ImageSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "Optional: what to analyze or look for in the image",
			},
			"image": map[string]any{
				"type":        "string",
				"description": "Image URL or data URI (http/https, mxc://, or data:)",
			},
			"image_url": map[string]any{
				"type":        "string",
				"description": "Deprecated alias for image",
			},
			"imageUrl": map[string]any{
				"type":        "string",
				"description": "OpenClaw-style alias for image",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional: model override (ignored in bridge)",
			},
			"maxBytesMb": map[string]any{
				"type":        "number",
				"description": "Optional: max image size in MB (ignored in bridge)",
			},
		},
		"required": []string{"image"},
	}
}

// ImageGenerateSchema returns the JSON schema for the image generation tool.
func ImageGenerateSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider": map[string]any{
				"type":        "string",
				"description": "Optional: provider override (openai, gemini, openrouter). Defaults to an available provider for this login.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The text prompt describing the image to generate",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional: image model to use (provider-specific)",
			},
			"count": map[string]any{
				"type":        "number",
				"description": "Optional: number of images to generate (default: 1).",
				"minimum":     1,
				"maximum":     10,
			},
			"size": map[string]any{
				"type":        "string",
				"description": "Optional: image size (OpenAI). Examples: 1024x1024, 1536x1024, 1024x1536, 1792x1024.",
			},
			"quality": map[string]any{
				"type":        "string",
				"description": "Optional: image quality (OpenAI). Examples: high, medium, low, standard, hd.",
			},
			"style": map[string]any{
				"type":        "string",
				"description": "Optional: image style (OpenAI DALL·E 3). Examples: vivid, natural.",
			},
			"background": map[string]any{
				"type":        "string",
				"description": "Optional: background mode (OpenAI GPT image models). Examples: transparent, opaque, auto.",
			},
			"output_format": map[string]any{
				"type":        "string",
				"description": "Optional: output format (OpenAI GPT image models). Examples: png, jpeg, webp.",
			},
			"outputFormat": map[string]any{
				"type":        "string",
				"description": "Optional: alias for output_format.",
			},
			"resolution": map[string]any{
				"type":        "string",
				"description": "Optional: output resolution (Gemini). Examples: 1K, 2K, 4K.",
			},
			"input_images": map[string]any{
				"type":        "array",
				"description": "Optional: input image paths/URLs/data URIs for editing/composition (Gemini).",
				"items": map[string]any{
					"type": "string",
				},
			},
			"inputImages": map[string]any{
				"type":        "array",
				"description": "Optional: alias for input_images.",
				"items": map[string]any{
					"type": "string",
				},
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
	return ImageSchema()
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
