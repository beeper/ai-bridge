package turns

import (
	"context"
	"testing"
	"time"

	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func TestStreamSessionEmitPartUsesResolvedRelationTarget(t *testing.T) {
	t.Helper()

	var gotContent map[string]any
	session := NewStreamSession(StreamSessionParams{
		TurnID:  "turn-1",
		AgentID: "agent-1",
		GetStreamTarget: func() StreamTarget {
			return StreamTarget{NetworkMessageID: networkid.MessageID("msg-1")}
		},
		ResolveTargetEventID: func(context.Context, StreamTarget) (id.EventID, error) {
			return id.EventID("$event-1"), nil
		},
		GetRoomID: func() id.RoomID {
			return id.RoomID("!room:example.com")
		},
		NextSeq: func() int { return 1 },
		SendHook: func(_ string, _ int, content map[string]any, _ string) bool {
			gotContent = content
			return true
		},
	})

	session.EmitPart(context.Background(), map[string]any{"type": "text-delta", "delta": "hello"})

	if gotContent == nil {
		t.Fatal("expected stream content to be emitted")
	}
	relatesTo, ok := gotContent["m.relates_to"].(map[string]any)
	if !ok {
		t.Fatalf("expected m.relates_to, got %#v", gotContent)
	}
	if relatesTo["event_id"] != "$event-1" {
		t.Fatalf("unexpected relation target: %#v", relatesTo)
	}
}

func TestStreamSessionFallsBackToDebouncedWithoutResolvedEventID(t *testing.T) {
	t.Helper()

	debounced := make(chan struct{}, 1)
	session := NewStreamSession(StreamSessionParams{
		TurnID: "turn-2",
		GetStreamTarget: func() StreamTarget {
			return StreamTarget{NetworkMessageID: networkid.MessageID("msg-2")}
		},
		ResolveTargetEventID: func(context.Context, StreamTarget) (id.EventID, error) {
			return "", nil
		},
		GetRoomID: func() id.RoomID {
			return id.RoomID("!room:example.com")
		},
		NextSeq: func() int { return 1 },
		SendDebouncedEdit: func(context.Context, bool) error {
			debounced <- struct{}{}
			return nil
		},
		SendHook: func(_ string, _ int, _ map[string]any, _ string) bool {
			t.Fatal("did not expect hook send when target event is unresolved")
			return false
		},
	})
	defer session.End(context.Background(), EndReasonFinish)

	session.EmitPart(context.Background(), map[string]any{"type": "finish"})

	select {
	case <-debounced:
	case <-time.After(2 * time.Second):
		t.Fatal("expected debounced fallback send")
	}
}

func TestStreamSessionDoesNothingWithoutEditTarget(t *testing.T) {
	t.Helper()

	called := make(chan struct{}, 1)
	session := NewStreamSession(StreamSessionParams{
		TurnID: "turn-3",
		GetStreamTarget: func() StreamTarget {
			return StreamTarget{}
		},
		ResolveTargetEventID: func(context.Context, StreamTarget) (id.EventID, error) {
			t.Fatal("did not expect target resolution without an edit target")
			return "", nil
		},
		SendDebouncedEdit: func(context.Context, bool) error {
			called <- struct{}{}
			return nil
		},
		SendHook: func(_ string, _ int, _ map[string]any, _ string) bool {
			called <- struct{}{}
			return true
		},
	})
	defer session.End(context.Background(), EndReasonFinish)

	session.EmitPart(context.Background(), map[string]any{"type": "finish"})

	select {
	case <-called:
		t.Fatal("did not expect stream send without an edit target")
	case <-time.After(150 * time.Millisecond):
	}
}
