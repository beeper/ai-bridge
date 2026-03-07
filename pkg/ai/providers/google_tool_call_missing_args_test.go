package providers

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestNormalizeGoogleToolCall_DefaultsMissingArgsToEmptyObject(t *testing.T) {
	toolCall := NormalizeGoogleToolCall("get_status", nil, "call_1", "")
	if toolCall.Type != ai.ContentTypeToolCall {
		t.Fatalf("expected tool call block type, got %s", toolCall.Type)
	}
	if toolCall.Name != "get_status" {
		t.Fatalf("unexpected tool name: %s", toolCall.Name)
	}
	if toolCall.Arguments == nil {
		t.Fatalf("expected arguments map, got nil")
	}
	if len(toolCall.Arguments) != 0 {
		t.Fatalf("expected empty arguments map, got %#v", toolCall.Arguments)
	}
}
