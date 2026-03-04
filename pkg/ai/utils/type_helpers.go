package utils

type StringEnumSchema struct {
	Type        string   `json:"type"`
	Enum        []string `json:"enum"`
	Description string   `json:"description,omitempty"`
	Default     string   `json:"default,omitempty"`
}

func StringEnum(values []string, description string, defaultValue string) map[string]any {
	schema := StringEnumSchema{
		Type:        "string",
		Enum:        append([]string(nil), values...),
		Description: description,
		Default:     defaultValue,
	}
	out := map[string]any{
		"type": "string",
		"enum": schema.Enum,
	}
	if schema.Description != "" {
		out["description"] = schema.Description
	}
	if schema.Default != "" {
		out["default"] = schema.Default
	}
	return out
}
