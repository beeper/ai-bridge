package tools

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
)

// CleanSchemaForProvider adjusts tool schemas for specific providers.
// Gemini requires flattened schemas without $ref, allOf, anyOf, oneOf.
// Based on clawdbot's clean-for-gemini pattern.
func CleanSchemaForProvider(schema map[string]any, provider string) map[string]any {
	switch provider {
	case "google", "gemini", "vertex":
		return cleanForGemini(schema)
	default:
		return schema
	}
}

// cleanForGemini removes unsupported schema constructs for Google's API.
func cleanForGemini(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	cleaned := make(map[string]any)
	for k, v := range schema {
		// Remove $ref, allOf, anyOf, oneOf - Gemini doesn't support these
		switch k {
		case "$ref", "allOf", "anyOf", "oneOf", "$schema", "$id":
			continue
		case "format":
			// Gemini is strict about format; remove to avoid errors
			continue
		case "additionalProperties":
			// Gemini sometimes has issues with this
			if b, ok := v.(bool); ok && !b {
				continue
			}
		}

		// Recursively clean nested objects
		if nested, ok := v.(map[string]any); ok {
			cleaned[k] = cleanForGemini(nested)
		} else if arr, ok := v.([]any); ok {
			// Clean array items
			cleanedArr := make([]any, len(arr))
			for i, item := range arr {
				if m, ok := item.(map[string]any); ok {
					cleanedArr[i] = cleanForGemini(m)
				} else {
					cleanedArr[i] = item
				}
			}
			cleaned[k] = cleanedArr
		} else {
			cleaned[k] = v
		}
	}
	return cleaned
}

// FlattenSchema resolves $ref and combines allOf into a single schema.
// Used before sending to providers that don't support JSON Schema references.
func FlattenSchema(schema map[string]any, definitions map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	// Handle $ref
	if ref, ok := schema["$ref"].(string); ok {
		resolved := resolveRef(ref, definitions)
		if resolved != nil {
			// Merge with any other properties in the schema
			merged := make(map[string]any)
			maps.Copy(merged, resolved)
			for k, v := range schema {
				if k != "$ref" {
					merged[k] = v
				}
			}
			return FlattenSchema(merged, definitions)
		}
	}

	// Handle allOf
	if allOf, ok := schema["allOf"].([]any); ok {
		merged := make(map[string]any)
		for _, item := range allOf {
			if m, ok := item.(map[string]any); ok {
				flat := FlattenSchema(m, definitions)
				mergeSchemas(merged, flat)
			}
		}
		// Also merge any other properties
		for k, v := range schema {
			if k != "allOf" {
				merged[k] = v
			}
		}
		return merged
	}

	// Recursively flatten nested schemas
	result := make(map[string]any)
	for k, v := range schema {
		if nested, ok := v.(map[string]any); ok {
			result[k] = FlattenSchema(nested, definitions)
		} else if arr, ok := v.([]any); ok {
			flatArr := make([]any, len(arr))
			for i, item := range arr {
				if m, ok := item.(map[string]any); ok {
					flatArr[i] = FlattenSchema(m, definitions)
				} else {
					flatArr[i] = item
				}
			}
			result[k] = flatArr
		} else {
			result[k] = v
		}
	}
	return result
}

// resolveRef resolves a JSON Schema $ref to its definition.
func resolveRef(ref string, definitions map[string]any) map[string]any {
	// Handle #/definitions/Name or #/$defs/Name format
	if len(ref) < 2 || ref[0] != '#' {
		return nil
	}

	// Parse path like #/definitions/Name
	// For simplicity, handle common patterns
	if len(ref) > 14 && ref[:14] == "#/definitions/" {
		name := ref[14:]
		if def, ok := definitions[name].(map[string]any); ok {
			return def
		}
	}
	if len(ref) > 9 && ref[:9] == "#/$defs/" {
		name := ref[9:]
		if def, ok := definitions[name].(map[string]any); ok {
			return def
		}
	}

	return nil
}

// mergeSchemas merges src into dst.
func mergeSchemas(dst, src map[string]any) {
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			// For properties and required, merge rather than overwrite
			if k == "properties" {
				if existMap, ok := existing.(map[string]any); ok {
					if srcMap, ok := v.(map[string]any); ok {
						maps.Copy(existMap, srcMap)
						continue
					}
				}
			}
			if k == "required" {
				if existArr, ok := existing.([]any); ok {
					if srcArr, ok := v.([]any); ok {
						dst[k] = append(existArr, srcArr...)
						continue
					}
				}
			}
		}
		dst[k] = v
	}
}

// ValidateInput validates tool input against a JSON schema.
// Returns nil if valid, error with details if invalid.
func ValidateInput(input map[string]any, schema map[string]any) error {
	if schema == nil {
		return nil
	}

	// Check required fields
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			name, ok := r.(string)
			if !ok {
				continue
			}
			if _, exists := input[name]; !exists {
				return fmt.Errorf("missing required parameter: %s", name)
			}
		}
	}

	// Check properties
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}

	for name, value := range input {
		propSchema, ok := properties[name].(map[string]any)
		if !ok {
			// Unknown property - could be additional properties
			continue
		}

		if err := validateValue(name, value, propSchema); err != nil {
			return err
		}
	}

	return nil
}

// validateValue validates a single value against its schema.
func validateValue(name string, value any, schema map[string]any) error {
	if value == nil {
		return nil // null is generally acceptable
	}

	expectedType, ok := schema["type"].(string)
	if !ok {
		return nil // No type constraint
	}

	actualType := getJSONType(value)

	// Type coercion for common cases
	switch expectedType {
	case "string":
		if actualType != "string" {
			return fmt.Errorf("parameter %s: expected string, got %s", name, actualType)
		}
	case "number", "integer":
		if actualType != "number" {
			return fmt.Errorf("parameter %s: expected %s, got %s", name, expectedType, actualType)
		}
		if expectedType == "integer" {
			if n, ok := value.(float64); ok && n != float64(int(n)) {
				return fmt.Errorf("parameter %s: expected integer, got float", name)
			}
		}
	case "boolean":
		if actualType != "boolean" {
			return fmt.Errorf("parameter %s: expected boolean, got %s", name, actualType)
		}
	case "array":
		if actualType != "array" {
			return fmt.Errorf("parameter %s: expected array, got %s", name, actualType)
		}
	case "object":
		if actualType != "object" {
			return fmt.Errorf("parameter %s: expected object, got %s", name, actualType)
		}
	}

	// Check enum constraint
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if reflect.DeepEqual(e, value) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("parameter %s: value not in allowed enum", name)
		}
	}

	return nil
}

// getJSONType returns the JSON type name for a Go value.
func getJSONType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64, float32, int, int64, int32:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

// SchemaToJSON converts a schema map to JSON string.
func SchemaToJSON(schema map[string]any) string {
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
