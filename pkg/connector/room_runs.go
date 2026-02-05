package connector

import (
	"context"

	"maunium.net/go/mautrix/id"
)

func (oc *AIClient) attachRoomRun(ctx context.Context, roomID id.RoomID) context.Context {
	if oc == nil || roomID == "" {
		return ctx
	}
	runCtx, cancel := context.WithCancel(ctx)
	oc.activeRoomCancelsMu.Lock()
	if oc.activeRoomCancels == nil {
		oc.activeRoomCancels = make(map[id.RoomID]context.CancelFunc)
	}
	oc.activeRoomCancels[roomID] = cancel
	oc.activeRoomCancelsMu.Unlock()
	return runCtx
}

func (oc *AIClient) cancelRoomRun(roomID id.RoomID) bool {
	if oc == nil || roomID == "" {
		return false
	}
	oc.activeRoomCancelsMu.Lock()
	cancel := oc.activeRoomCancels[roomID]
	oc.activeRoomCancelsMu.Unlock()
	if cancel != nil {
		cancel()
		return true
	}
	return false
}

func (oc *AIClient) clearRoomRun(roomID id.RoomID) {
	if oc == nil || roomID == "" {
		return
	}
	oc.activeRoomCancelsMu.Lock()
	if cancel, ok := oc.activeRoomCancels[roomID]; ok {
		delete(oc.activeRoomCancels, roomID)
		if cancel != nil {
			cancel()
		}
	}
	oc.activeRoomCancelsMu.Unlock()
}
