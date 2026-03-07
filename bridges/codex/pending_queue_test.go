package codex

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestCodexPendingQueueFIFO(t *testing.T) {
	roomID := id.RoomID("!room:example.com")
	cc := &CodexClient{
		activeRooms:     make(map[id.RoomID]bool),
		pendingMessages: make(map[id.RoomID]codexPendingQueue),
	}

	first := &codexPendingMessage{body: "first"}
	second := &codexPendingMessage{body: "second"}

	cc.queuePendingCodex(roomID, first)
	cc.queuePendingCodex(roomID, second)

	if got := cc.popPendingCodex(roomID); got != first {
		t.Fatalf("expected first pending message, got %#v", got)
	}
	if got := cc.popPendingCodex(roomID); got != second {
		t.Fatalf("expected second pending message, got %#v", got)
	}
	if got := cc.popPendingCodex(roomID); got != nil {
		t.Fatalf("expected queue to be empty, got %#v", got)
	}
}

func TestCodexAcquireRoomIfQueueEmpty(t *testing.T) {
	roomID := id.RoomID("!room:example.com")
	cc := &CodexClient{
		activeRooms:     make(map[id.RoomID]bool),
		pendingMessages: make(map[id.RoomID]codexPendingQueue),
	}

	cc.queuePendingCodex(roomID, &codexPendingMessage{body: "queued"})
	if cc.acquireRoomIfQueueEmpty(roomID) {
		t.Fatalf("expected queued room to remain unavailable for immediate dispatch")
	}

	if got := cc.beginPendingCodex(roomID); got == nil || got.body != "queued" {
		t.Fatalf("expected beginPendingCodex to reserve queued head, got %#v", got)
	}
	if !cc.activeRooms[roomID] {
		t.Fatalf("expected beginPendingCodex to reserve the room")
	}
	cc.releaseRoom(roomID)
}
