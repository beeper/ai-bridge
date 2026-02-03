package toolspec

const (
	StatName        = "stat"
	StatDescription = "Get metadata for a file or directory (type, size, hash, updatedAt, entries)."
)

// StatSchema returns the JSON schema for the stat tool.
func StatSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to file or directory to stat.",
			},
		},
		"required": []string{"path"},
	}
}
