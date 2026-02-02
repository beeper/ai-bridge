package connector

import "testing"

func TestSanitizeToolSchema_StripsUnsupportedKeywords(t *testing.T) {
	schema := map[string]any{
		"type":          "object",
		"minProperties": 1,
		"properties": map[string]any{
			"title": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}

	cleaned := sanitizeToolSchema(schema)
	if _, ok := cleaned["minProperties"]; ok {
		t.Fatalf("expected minProperties to be stripped")
	}
	props, ok := cleaned["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to remain")
	}
	title, ok := props["title"].(map[string]any)
	if !ok {
		t.Fatalf("expected title property to remain")
	}
	if _, ok := title["minLength"]; ok {
		t.Fatalf("expected minLength to be stripped from nested property")
	}
}

func TestSanitizeToolSchema_ConvertsConstToEnum(t *testing.T) {
	schema := map[string]any{
		"const": "send",
	}
	cleaned := sanitizeToolSchema(schema)
	if _, ok := cleaned["const"]; ok {
		t.Fatalf("expected const to be removed")
	}
	enumVals, ok := cleaned["enum"].([]any)
	if !ok || len(enumVals) != 1 || enumVals[0] != "send" {
		t.Fatalf("expected enum to contain const value, got %+v", cleaned["enum"])
	}
}
