package connector

import "maunium.net/go/mautrix/id"

//lint:ignore U1000 Reserved for future UI/state integration.
func (oc *AIClient) isRoomBusy(roomID id.RoomID) bool {
	if oc == nil {
		return false
	}
	oc.activeRoomsMu.Lock()
	active := oc.activeRooms[roomID]
	oc.activeRoomsMu.Unlock()

	oc.pendingQueuesMu.Lock()
	queue := oc.pendingQueues[roomID]
	pending := queue != nil && (len(queue.items) > 0 || queue.droppedCount > 0)
	oc.pendingQueuesMu.Unlock()

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
	oc.pendingQueuesMu.Lock()
	for _, queue := range oc.pendingQueues {
		if queue != nil && (len(queue.items) > 0 || queue.droppedCount > 0) {
			pending = true
			break
		}
	}
	oc.pendingQueuesMu.Unlock()

	return active || pending
}
