package connector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	aipkg "github.com/beeper/ai-bridge/pkg/ai"
)

func TestAIUsageToConnectorUsage(t *testing.T) {
	if usage := aiUsageToConnectorUsage(aipkg.Usage{}); usage != nil {
		t.Fatalf("expected nil usage for all-zero usage input")
	}

	usage := aiUsageToConnectorUsage(aipkg.Usage{
		Input:       100,
		Output:      40,
		CacheRead:   20,
		CacheWrite:  10,
		TotalTokens: 120,
	})
	if usage == nil {
		t.Fatalf("expected mapped usage")
	}
	if usage.PromptTokens != 130 {
		t.Fatalf("expected prompt tokens input+cache=130, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 40 {
		t.Fatalf("expected completion tokens 40, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 170 {
		t.Fatalf("expected total tokens uplifted to computed sum 170, got %d", usage.TotalTokens)
	}
}

func TestAIEventToStreamEvent_Mapping(t *testing.T) {
	if evt, ok := aiEventToStreamEvent(aipkg.AssistantMessageEvent{
		Type:  aipkg.EventTextDelta,
		Delta: "abc",
	}); !ok || evt.Type != StreamEventDelta || evt.Delta != "abc" {
		t.Fatalf("unexpected text delta mapping: ok=%v evt=%#v", ok, evt)
	}

	if evt, ok := aiEventToStreamEvent(aipkg.AssistantMessageEvent{
		Type:  aipkg.EventThinkingDelta,
		Delta: "reason",
	}); !ok || evt.Type != StreamEventReasoning || evt.ReasoningDelta != "reason" {
		t.Fatalf("unexpected thinking delta mapping: ok=%v evt=%#v", ok, evt)
	}

	toolEvent, ok := aiEventToStreamEvent(aipkg.AssistantMessageEvent{
		Type: aipkg.EventToolCallEnd,
		ToolCall: &aipkg.ContentBlock{
			Type:      aipkg.ContentTypeToolCall,
			ID:        "call_1",
			Name:      "search",
			Arguments: map[string]any{"q": "golang"},
		},
	})
	if !ok || toolEvent.Type != StreamEventToolCall || toolEvent.ToolCall == nil {
		t.Fatalf("unexpected tool call mapping: ok=%v evt=%#v", ok, toolEvent)
	}
	args := map[string]any{}
	if err := json.Unmarshal([]byte(toolEvent.ToolCall.Arguments), &args); err != nil {
		t.Fatalf("expected tool args JSON, got err=%v", err)
	}
	if args["q"] != "golang" {
		t.Fatalf("expected tool arg q=golang, got %#v", args)
	}

	doneEvent, ok := aiEventToStreamEvent(aipkg.AssistantMessageEvent{
		Type:   aipkg.EventDone,
		Reason: aipkg.StopReasonStop,
		Message: aipkg.Message{
			Usage: aipkg.Usage{
				Input:      10,
				Output:     5,
				CacheRead:  2,
				CacheWrite: 1,
			},
		},
	})
	if !ok || doneEvent.Type != StreamEventComplete || doneEvent.FinishReason != "stop" {
		t.Fatalf("unexpected done mapping: ok=%v evt=%#v", ok, doneEvent)
	}
	if doneEvent.Usage == nil || doneEvent.Usage.TotalTokens != 18 {
		t.Fatalf("expected mapped usage total=18, got %#v", doneEvent.Usage)
	}

	errEvent, ok := aiEventToStreamEvent(aipkg.AssistantMessageEvent{
		Type:  aipkg.EventError,
		Error: aipkg.Message{ErrorMessage: "boom"},
	})
	if !ok || errEvent.Type != StreamEventError || errEvent.Error == nil || errEvent.Error.Error() != "boom" {
		t.Fatalf("unexpected error mapping: ok=%v evt=%#v", ok, errEvent)
	}
}

func TestStreamEventsFromAIStream(t *testing.T) {
	stream := aipkg.NewAssistantMessageEventStream(8)
	go func() {
		stream.Push(aipkg.AssistantMessageEvent{Type: aipkg.EventStart})
		stream.Push(aipkg.AssistantMessageEvent{Type: aipkg.EventTextDelta, Delta: "hello"})
		stream.Push(aipkg.AssistantMessageEvent{
			Type:   aipkg.EventDone,
			Reason: aipkg.StopReasonStop,
			Message: aipkg.Message{
				Usage: aipkg.Usage{Input: 1, Output: 1, TotalTokens: 2},
			},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out := streamEventsFromAIStream(ctx, stream)
	var collected []StreamEvent
	for evt := range out {
		collected = append(collected, evt)
	}
	if len(collected) != 2 {
		t.Fatalf("expected 2 mapped events (delta + complete), got %d", len(collected))
	}
	if collected[0].Type != StreamEventDelta || collected[0].Delta != "hello" {
		t.Fatalf("unexpected first mapped event: %#v", collected[0])
	}
	if collected[1].Type != StreamEventComplete || collected[1].FinishReason != "stop" {
		t.Fatalf("unexpected completion mapped event: %#v", collected[1])
	}
}
