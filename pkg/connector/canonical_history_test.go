package connector

import (
	"context"
	"testing"

	"github.com/beeper/ai-bridge/pkg/bridgeadapter"
)

func TestHistoryMessageBundle_LegacyAssistantFallback(t *testing.T) {
	oc := &AIClient{}
	bundle := oc.historyMessageBundle(context.Background(), &MessageMetadata{
		BaseMessageMetadata: bridgeadapter.BaseMessageMetadata{
			Role: "assistant",
			Body: "done",
			ToolCalls: []ToolCallMetadata{{
				CallID:   "call_1",
				ToolName: "Read",
				Input:    map[string]any{"path": "README.md"},
				Output:   map[string]any{"result": "ok"},
			}},
		},
	}, false)

	if len(bundle) != 2 {
		t.Fatalf("expected assistant bundle with tool output, got %d entries", len(bundle))
	}
	if bundle[0].Role != PromptRoleAssistant {
		t.Fatalf("expected first bundle entry to be assistant message")
	}
	if len(bundle[0].Blocks) != 2 || bundle[0].Blocks[1].Type != PromptBlockToolCall {
		t.Fatalf("expected assistant tool call block to be preserved, got %#v", bundle[0].Blocks)
	}
	if bundle[1].Role != PromptRoleToolResult || bundle[1].ToolCallID != "call_1" {
		t.Fatalf("expected tool output for call_1, got %#v", bundle[1])
	}
}

func TestHistoryMessageBundle_UsesLegacyMetadataOnly(t *testing.T) {
	oc := &AIClient{}
	bundle := oc.historyMessageBundle(context.Background(), &MessageMetadata{
		BaseMessageMetadata: bridgeadapter.BaseMessageMetadata{
			Role: "assistant",
			Body: "hello",
			ToolCalls: []ToolCallMetadata{{
				CallID:   "call_1",
				ToolName: "Read",
				Input:    map[string]any{"path": "README.md"},
				Output:   map[string]any{"result": "ok"},
			}},
		},
	}, false)

	if len(bundle) != 2 {
		t.Fatalf("expected assistant bundle with tool output, got %d entries", len(bundle))
	}
	if got := bundle[0].Text(); got != "hello" {
		t.Fatalf("expected assistant text hello, got %q", got)
	}
	if bundle[0].Blocks[1].Type != PromptBlockToolCall {
		t.Fatalf("expected tool call block, got %#v", bundle[0].Blocks)
	}
	if bundle[1].Role != PromptRoleToolResult || bundle[1].ToolCallID != "call_1" {
		t.Fatalf("expected tool output for call_1, got %#v", bundle[1])
	}
}

func TestHistoryMessageBundle_AudioHistoryStaysTextOnly(t *testing.T) {
	oc := &AIClient{}
	bundle := oc.historyMessageBundle(context.Background(), &MessageMetadata{
		BaseMessageMetadata: bridgeadapter.BaseMessageMetadata{
			Role: "user",
			Body: "Transcript: hello world",
		},
		MediaURL: "mxc://example/audio",
		MimeType: "audio/mpeg",
	}, false)

	if len(bundle) != 1 {
		t.Fatalf("expected one user message, got %d", len(bundle))
	}
	if bundle[0].Role != PromptRoleUser {
		t.Fatalf("expected user prompt message, got %#v", bundle[0])
	}
	if len(bundle[0].Blocks) != 1 || bundle[0].Blocks[0].Type != PromptBlockText {
		t.Fatalf("expected text-only audio history, got %#v", bundle[0].Blocks)
	}
	if got := bundle[0].Blocks[0].Text; got != "Transcript: hello world\n[mxc://example/audio]" && got != "Transcript: hello world\n[media_url: mxc://example/audio]" {
		t.Fatalf("expected transcript plus media marker, got %q", got)
	}
}
