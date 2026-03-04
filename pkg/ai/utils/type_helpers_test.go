package utils

import "testing"

func TestStringEnum(t *testing.T) {
	schema := StringEnum([]string{"add", "subtract"}, "operation", "add")
	if schema["type"] != "string" {
		t.Fatalf("expected type string, got %v", schema["type"])
	}
	enumVals, ok := schema["enum"].([]string)
	if !ok || len(enumVals) != 2 {
		t.Fatalf("expected enum values in schema, got %#v", schema["enum"])
	}
	if schema["description"] != "operation" {
		t.Fatalf("expected description set")
	}
	if schema["default"] != "add" {
		t.Fatalf("expected default add")
	}
}
