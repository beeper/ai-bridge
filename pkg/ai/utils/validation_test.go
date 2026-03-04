package utils

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestValidateToolCallAndArguments(t *testing.T) {
	tool := ai.Tool{
		Name:        "calculate",
		Description: "calc",
		Parameters: map[string]any{
			"type": "object",
			"required": []any{
				"expression",
				"strict",
			},
			"properties": map[string]any{
				"expression": map[string]any{"type": "string"},
				"strict":     map[string]any{"type": "boolean"},
				"count":      map[string]any{"type": "number"},
			},
		},
	}
	call := ai.ContentBlock{
		Type: ai.ContentTypeToolCall,
		Name: "calculate",
		Arguments: map[string]any{
			"expression": 123,
			"strict":     "true",
			"count":      "10.5",
		},
	}
	validated, err := ValidateToolCall([]ai.Tool{tool}, call)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if validated["expression"] != "123" {
		t.Fatalf("expected expression coerced to string, got %#v", validated["expression"])
	}
	if validated["strict"] != true {
		t.Fatalf("expected strict coerced to bool, got %#v", validated["strict"])
	}
	if validated["count"] != 10.5 {
		t.Fatalf("expected count coerced to float64, got %#v", validated["count"])
	}
}

func TestValidateToolCall_MissingToolAndRequiredField(t *testing.T) {
	_, err := ValidateToolCall(nil, ai.ContentBlock{Name: "missing"})
	if err == nil {
		t.Fatalf("expected error for missing tool")
	}

	tool := ai.Tool{
		Name: "echo",
		Parameters: map[string]any{
			"type": "object",
			"required": []any{
				"message",
			},
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}
	_, err = ValidateToolCall([]ai.Tool{tool}, ai.ContentBlock{
		Name:      "echo",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Fatalf("expected missing required field error")
	}
}
