package connector

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
