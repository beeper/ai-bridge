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

func (oc *AIClient) hasInflightRequests() bool {
	if oc == nil {
		return false
	}
	active := false
	oc.activeRoomsMu.Lock()
	for _, inFlight := range oc.activeRooms {
		if inFlight {
			active = true
			break
		}
	}
	oc.activeRoomsMu.Unlock()

	pending := false
	oc.pendingMessagesMu.Lock()
	for _, queue := range oc.pendingMessages {
		if len(queue) > 0 {
			pending = true
			break
		}
	}
	oc.pendingMessagesMu.Unlock()

	return active || pending
}
