package toolspec

const (
	LSName        = "ls"
	LSDescription = "List directory contents. Includes dotfiles."

	FindName        = "find"
	FindDescription = "Search for files by glob pattern. Respects .gitignore."

	GrepName        = "grep"
	GrepDescription = "Search file contents (regex or literal). Respects .gitignore."
)

func LSSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list (defaults to workspace root).",
			},
		},
	}
}

func FindSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match file paths (e.g., '*.go', 'pkg/**/tools*.go').",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional base directory to search within.",
			},
		},
		"required": []string{"pattern"},
	}
}

func GrepSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Pattern to search for (regex or literal).",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional file or directory path to search within.",
			},
			"regex": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as regex (default false).",
			},
			"caseSensitive": map[string]any{
				"type":        "boolean",
				"description": "Case-sensitive search (default false).",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Maximum number of matches to return (default 20).",
			},
		},
		"required": []string{"pattern"},
	}
}
