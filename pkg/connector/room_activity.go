package connector

import "maunium.net/go/mautrix/id"

func (oc *AIClient) isRoomBusy(roomID id.RoomID) bool {
	if oc == nil {
		return false
	}
	oc.activeRoomsMu.Lock()
	active := oc.activeRooms[roomID]
	oc.activeRoomsMu.Unlock()

	oc.pendingMessagesMu.Lock()
	pending := len(oc.pendingMessages[roomID]) > 0
	oc.pendingMessagesMu.Unlock()

	return active || pending
}
