package toolspec

// Shared tool schema definitions used by both connector and agents.

const (
	CalculatorName        = "calculator"
	CalculatorDescription = "Perform basic arithmetic calculations. Supports addition, subtraction, multiplication, division, and modulo operations."

	WebSearchName        = "web_search"
	WebSearchDescription = "Search the web using Brave Search API. Supports region-specific and localized search via country and language parameters. Returns titles, URLs, and snippets for fast research."

	WebSearchOpenRouterName        = "web_search_openrouter"
	WebSearchOpenRouterDescription = "Search the web using Perplexity Sonar (direct or via OpenRouter). Returns AI-synthesized answers with citations from real-time web search."

	WebFetchName        = "web_fetch"
	WebFetchDescription = "Fetch and extract readable content from a URL (HTML \u2192 markdown/text). Use for lightweight page access without browser automation."

	MessageName        = "message"
	MessageDescription = "Send, delete, and manage messages via channel plugins. Supports actions: send, delete, react, poll, pin, threads, and more."

	SessionStatusName        = "session_status"
	SessionStatusDescription = "Show a /status-equivalent session status card (usage + time + cost when available). Use for model-use questions (📊 session_status). Optional: set per-session model override (model=default resets overrides)."

	// ImageName matches OpenClaw's image analysis tool (vision).
	ImageName        = "image"
	ImageDescription = "Analyze an image with the configured image model (agents.defaults.imageModel). Provide a prompt and image path or URL."

	// ImageGenerateName is an AI image generation tool (not in OpenClaw).
	ImageGenerateName        = "image_generate"
	ImageGenerateDescription = "Generate one or more images from a text prompt. Supports provider-specific controls such as size, quality, style, background, output format, resolution, and optional input images for editing/composition."

	TTSName        = "tts"
	TTSDescription = "Convert text to speech and return a MEDIA: path. Use when the user requests audio or TTS is enabled. Copy the MEDIA line exactly."

	// AnalyzeImageName is a deprecated alias for ImageName.
	AnalyzeImageName        = "analyze_image"
	AnalyzeImageDescription = "Deprecated alias for image (vision analysis)."

	MemorySearchName        = "memory_search"
	MemorySearchDescription = "Mandatory recall step: semantically search MEMORY.md + memory/*.md (and optional session transcripts) before answering questions about prior work, decisions, dates, people, preferences, or todos; returns top snippets with path + lines."
	MemoryGetName           = "memory_get"
	MemoryGetDescription    = "Safe snippet read from MEMORY.md, memory/*.md, or configured memorySearch.extraPaths with optional from/lines; use after memory_search to pull only the needed lines and keep context small."

	ReadName         = "read"
	ReadDescription  = "Read a text file from the virtual workspace. Supports offset/limit for large files."
	WriteName        = "write"
	WriteDescription = "Write content to a text file in the virtual workspace. Creates or overwrites the file."
	EditName         = "edit"
	EditDescription  = "Edit a text file by replacing exact text (oldText -> newText)."
	LsName           = "ls"
	LsDescription    = "List directory contents in the virtual workspace."
	FindName         = "find"
	FindDescription  = "Search for files by glob pattern in the virtual workspace."
	GrepName         = "grep"
	GrepDescription  = "Search file contents for a pattern in the virtual workspace."

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

// ReadSchema returns the JSON schema for the read tool.
func ReadSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read (relative to the virtual workspace)",
			},
			"file_path": map[string]any{
				"type":        "string",
				"description": "OpenClaw/Claude-style alias for path",
			},
			"offset": map[string]any{
				"type":        "number",
				"description": "Line number to start reading from (1-indexed)",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Maximum number of lines to read",
			},
		},
		"required": []string{"path"},
	}
}

// WriteSchema returns the JSON schema for the write tool.
func WriteSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write (relative to the virtual workspace)",
			},
			"file_path": map[string]any{
				"type":        "string",
				"description": "OpenClaw/Claude-style alias for path",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

// EditSchema returns the JSON schema for the edit tool.
func EditSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit (relative to the virtual workspace)",
			},
			"file_path": map[string]any{
				"type":        "string",
				"description": "OpenClaw/Claude-style alias for path",
			},
			"oldText": map[string]any{
				"type":        "string",
				"description": "Exact text to find and replace (must match exactly)",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "OpenClaw/Claude-style alias for oldText",
			},
			"newText": map[string]any{
				"type":        "string",
				"description": "Replacement text",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "OpenClaw/Claude-style alias for newText",
			},
		},
		"required": []string{"path", "oldText", "newText"},
	}
}

// LsSchema returns the JSON schema for the ls tool.
func LsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to list (default: root)",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Maximum number of entries to return (default: 500)",
			},
		},
	}
}

// FindSchema returns the JSON schema for the find tool.
func FindSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files (e.g. '*.md', '**/*.json')",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in (default: root)",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Maximum number of results (default: 1000)",
			},
		},
		"required": []string{"pattern"},
	}
}

// GrepSchema returns the JSON schema for the grep tool.
func GrepSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Search pattern (regex or literal string)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory or file to search (default: root)",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Filter files by glob pattern, e.g. '*.ts' or '**/*.md'",
			},
			"ignoreCase": map[string]any{
				"type":        "boolean",
				"description": "Case-insensitive search (default: false)",
			},
			"literal": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as literal string instead of regex (default: false)",
			},
			"context": map[string]any{
				"type":        "number",
				"description": "Number of lines to show before and after each match (default: 0)",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Maximum number of matches to return (default: 100)",
			},
		},
		"required": []string{"pattern"},
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
				"description": "Path to a memory file (e.g., 'MEMORY.md' or 'memory/2026-02-03.md')",
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
