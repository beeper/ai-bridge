package ai

import (
	"testing"

	"github.com/beeper/agentremote/pkg/shared/streamui"
	"github.com/beeper/agentremote/sdk"
)

func TestCanonicalPromptMessagesPrefersTurnData(t *testing.T) {
	meta := &MessageMetadata{}
	meta.CanonicalTurnSchema = sdk.CanonicalTurnDataSchemaV1
	meta.CanonicalTurnData = sdk.TurnData{
		ID:   "turn-1",
		Role: "assistant",
		Parts: []sdk.TurnPart{
			{Type: "text", Text: "hello"},
			{Type: "tool", ToolCallID: "call_1", ToolName: "search", Input: map[string]any{"query": "matrix"}, Output: map[string]any{"ok": true}},
		},
	}.ToMap()

	messages := canonicalPromptMessages(meta)
	if len(messages) != 2 {
		t.Fatalf("expected assistant + tool result, got %d messages", len(messages))
	}
	if messages[0].Role != PromptRoleAssistant {
		t.Fatalf("expected assistant role, got %q", messages[0].Role)
	}
	if messages[1].Role != PromptRoleToolResult {
		t.Fatalf("expected tool result role, got %q", messages[1].Role)
	}
}

func TestSetCanonicalPromptMessagesStoresTurnDataForUser(t *testing.T) {
	meta := &MessageMetadata{}
	setCanonicalPromptMessages(meta, []PromptMessage{{
		Role: PromptRoleUser,
		Blocks: []PromptBlock{{
			Type: PromptBlockText,
			Text: "hello",
		}},
	}})

	if meta.CanonicalTurnSchema != sdk.CanonicalTurnDataSchemaV1 {
		t.Fatalf("expected turn data schema, got %q", meta.CanonicalTurnSchema)
	}
	td, ok := canonicalTurnData(meta)
	if !ok {
		t.Fatalf("expected canonical turn data")
	}
	if td.Role != "user" || len(td.Parts) != 1 || td.Parts[0].Text != "hello" {
		t.Fatalf("unexpected turn data: %#v", td)
	}
}

func TestCanonicalPromptMessagesFallsBackWhenTurnDataProjectionIsEmpty(t *testing.T) {
	meta := &MessageMetadata{}
	meta.CanonicalTurnSchema = sdk.CanonicalTurnDataSchemaV1
	meta.CanonicalTurnData = sdk.TurnData{
		ID:   "turn-1",
		Role: "",
		Parts: []sdk.TurnPart{
			{Type: "text", Text: "dropped"},
		},
	}.ToMap()
	meta.CanonicalPromptSchema = canonicalPromptSchemaV1
	meta.CanonicalPromptMessages = encodePromptMessages([]PromptMessage{{
		Role: PromptRoleUser,
		Blocks: []PromptBlock{{
			Type: PromptBlockText,
			Text: "fallback",
		}},
	}})

	messages := canonicalPromptMessages(meta)
	if len(messages) != 1 {
		t.Fatalf("expected 1 fallback message, got %d", len(messages))
	}
	if messages[0].Role != PromptRoleUser {
		t.Fatalf("expected fallback user role, got %q", messages[0].Role)
	}
	if got := messages[0].Text(); got != "fallback" {
		t.Fatalf("expected fallback text, got %q", got)
	}
}

func TestTurnDataFromStreamingStatePrefersVisibleText(t *testing.T) {
	state := testStreamingState("turn-visible")
	state.accumulated.WriteString("[[reply_to_current]] hidden")
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "start", "messageId": "turn-visible"})
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "text-start", "id": "text-visible"})
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "text-delta", "id": "text-visible", "delta": "Visible reply"})
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "text-end", "id": "text-visible"})

	td := turnDataFromStreamingState(state, streamui.SnapshotCanonicalUIMessage(state.turn.UIState()))
	if len(td.Parts) == 0 || td.Parts[0].Text != "Visible reply" {
		t.Fatalf("expected visible turn text in first part, got %#v", td.Parts)
	}
}

func TestAssistantPromptMessagesFromStatePrefersVisibleText(t *testing.T) {
	state := testStreamingState("turn-prompt-visible")
	state.accumulated.WriteString("[[reply_to_current]] hidden")
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "start", "messageId": "turn-prompt-visible"})
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "text-start", "id": "text-prompt-visible"})
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "text-delta", "id": "text-prompt-visible", "delta": "Visible prompt text"})
	streamui.ApplyChunk(state.turn.UIState(), map[string]any{"type": "text-end", "id": "text-prompt-visible"})

	messages := assistantPromptMessagesFromState(state)
	if len(messages) != 1 {
		t.Fatalf("expected one assistant prompt message, got %d", len(messages))
	}
	if got := messages[0].Text(); got != "Visible prompt text" {
		t.Fatalf("expected visible prompt text, got %q", got)
	}
}
