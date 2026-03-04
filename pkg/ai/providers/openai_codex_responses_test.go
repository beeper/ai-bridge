package providers

import (
	"strings"
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestProcessCodexSSEPayload_MapsToAssistantEvents(t *testing.T) {
	payload := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
		``,
		`data: {"type":"response.content_part.added","part":{"type":"output_text","text":""}}`,
		``,
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		``,
		`data: {"type":"response.output_item.done","item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8,"input_tokens_details":{"cached_tokens":0}}}}`,
		``,
	}, "\n")

	output := ai.Message{
		Role:     ai.RoleAssistant,
		API:      ai.APIOpenAICodexResponse,
		Provider: "openai-codex",
		Model:    "gpt-5.1-codex",
	}
	events, err := ProcessCodexSSEPayload(payload, &output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected events from payload")
	}

	var sawTextDelta, sawDone bool
	for _, evt := range events {
		if evt.Type == ai.EventTextDelta {
			sawTextDelta = true
			if evt.Delta != "Hello" {
				t.Fatalf("expected text delta Hello, got %q", evt.Delta)
			}
		}
		if evt.Type == ai.EventDone {
			sawDone = true
			if evt.Message.StopReason != ai.StopReasonStop {
				t.Fatalf("expected done stop reason stop, got %s", evt.Message.StopReason)
			}
		}
	}
	if !sawTextDelta {
		t.Fatalf("expected text delta event")
	}
	if !sawDone {
		t.Fatalf("expected done event")
	}
	if output.Usage.TotalTokens != 8 || output.Usage.Input != 5 || output.Usage.Output != 3 {
		t.Fatalf("unexpected usage: %+v", output.Usage)
	}
	if len(output.Content) != 1 || output.Content[0].Text != "Hello" {
		t.Fatalf("unexpected output content: %+v", output.Content)
	}
}
