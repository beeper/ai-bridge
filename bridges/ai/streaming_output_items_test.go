package ai

import (
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

func TestParseJSONOrRaw_EmptyStringReturnsNil(t *testing.T) {
	if got := parseJSONOrRaw(""); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
}

func TestDeriveToolDescriptorForOutputItem_FunctionCallLeavesBlankArgumentsEmpty(t *testing.T) {
	desc := deriveToolDescriptorForOutputItem(responses.ResponseOutputItemUnion{
		ID:        "item_123",
		CallID:    "call_123",
		Type:      "function_call",
		Name:      "web_search",
		Arguments: "",
	}, nil)

	if !desc.ok {
		t.Fatal("expected descriptor to be valid")
	}
	if desc.input != nil {
		t.Fatalf("expected blank arguments to remain nil, got %#v", desc.input)
	}
}

func TestDeriveToolDescriptorForOutputItem_FunctionCallParsesArgumentsJSON(t *testing.T) {
	desc := deriveToolDescriptorForOutputItem(responses.ResponseOutputItemUnion{
		ID:        "item_123",
		CallID:    "call_123",
		Type:      "function_call",
		Name:      "web_search",
		Arguments: "{\"query\":\"latest news headlines today\",\"count\":10}",
	}, nil)

	if !desc.ok {
		t.Fatal("expected descriptor to be valid")
	}
	input, ok := desc.input.(map[string]any)
	if !ok {
		t.Fatalf("expected parsed argument map, got %#v", desc.input)
	}
	if got := input["query"]; got != "latest news headlines today" {
		t.Fatalf("expected query to be preserved, got %#v", got)
	}
	if got := input["count"]; got != float64(10) {
		t.Fatalf("expected count 10, got %#v", got)
	}
}
