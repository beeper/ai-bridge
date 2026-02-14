package toolspec

// Compact local wrappers around Clay Nexus tools.
//
// These are NOT mirrored from clay-nexus; they exist to reduce tool list size
// for models by multiplexing multiple operations behind a single entrypoint.

const (
	// NexusContactsName is a compact multiplexer for contact-related Nexus tools.
	NexusContactsName        = "contacts"
	NexusContactsDescription = "Unified Clay/Nexus contacts tool. Set action to one of: search|get|create|update|note|find_duplicates. Pass the same arguments as the underlying Nexus tool for that action."
)

func NexusContactsSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					"search",
					"get",
					"create",
					"update",
					"note",
					"find_duplicates",
				},
				"description": "Which contacts operation to run.",
			},

			// Common contact args (superset; action decides which are required).
			"query": map[string]any{"type": "string"},
			"keywords": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"name": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"group_ids": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			},
			"limit": map[string]any{"type": "number"},
			"page":  map[string]any{"type": "number"},
			"exclude_contact_ids": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			},
			"sort": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"field":     map[string]any{"type": "string"},
					"direction": map[string]any{"type": "string", "enum": []string{"asc", "desc"}},
				},
			},

			"contact_id": map[string]any{"type": "number"},
			"contact_ids": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			},

			// create/update fields
			"first_name": map[string]any{"type": "string"},
			"last_name":  map[string]any{"type": "string"},
			"phone": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"email": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"linkedin":     map[string]any{"type": "string"},
			"bio":          map[string]any{"type": "string"},
			"title":        map[string]any{"type": "string"},
			"organization": map[string]any{"type": "string"},
			"birthday":     map[string]any{"type": "string"},
			"website": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},

			// notes
			"content":       map[string]any{"type": "string"},
			"reminder_date": map[string]any{"type": "string"},
		},
		"required": []string{"action"},
	}
}
