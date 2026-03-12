package agentremote

import (
	"testing"

	"github.com/beeper/agentremote/turns"
)

func TestTurnManagerLifecycle(t *testing.T) {
	runtime := NewRuntime(RuntimeConfig{AgentID: "assistant"})
	manager := runtime.Turns
	turn := manager.StartTurn(TurnOptions{ID: "turn-1"})
	if turn == nil {
		t.Fatal("expected turn")
	}
	if turn.AgentID != "assistant" {
		t.Fatalf("expected runtime agent id, got %q", turn.AgentID)
	}
	if got := manager.Get("turn-1"); got != turn {
		t.Fatalf("expected to retrieve started turn, got %#v", got)
	}

	turn.AttachSession(nil)
	turn.ApplyEvent(TurnEvent{
		Type: TurnEventMessageUpdate,
		Message: &AgentMessage{
			Role: RoleAssistant,
			Text: "hello",
		},
	})
	turn.ApplyEvent(TurnEvent{
		Type:     TurnEventEnd,
		Metadata: map[string]any{"finish_reason": "completed"},
	})

	snapshot := turn.SnapshotCopy()
	if snapshot.VisibleText != "hello" {
		t.Fatalf("expected visible text to accumulate assistant output, got %q", snapshot.VisibleText)
	}
	if snapshot.FirstTokenAtMs == 0 {
		t.Fatal("expected first token timestamp to be set")
	}
	if snapshot.FinishReason != "completed" {
		t.Fatalf("expected finish reason from event metadata, got %q", snapshot.FinishReason)
	}

	manager.End("turn-1", turns.EndReason("done"))
	if got := manager.Get("turn-1"); got != nil {
		t.Fatalf("expected turn to be removed after End, got %#v", got)
	}
}
