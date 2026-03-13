package helpers

import (
	"sync"
)

// MessageQueue serializes message processing per room, ensuring only one
// handler runs at a time for each room ID.
type MessageQueue struct {
	mu     sync.Mutex
	active map[string]chan struct{}
}

// NewMessageQueue creates a new MessageQueue.
func NewMessageQueue() *MessageQueue {
	return &MessageQueue{
		active: make(map[string]chan struct{}),
	}
}

// Enqueue runs handler for the given room, waiting for any in-progress handler
// to finish first. Multiple Enqueue calls for the same room are serialized.
func (q *MessageQueue) Enqueue(roomID string, handler func()) {
	q.acquireOrWait(roomID)
	defer q.ReleaseRoom(roomID)
	handler()
}

// AcquireRoom marks a room as active. Returns true if the room was not already
// active, false if it was (caller should wait or skip).
func (q *MessageQueue) AcquireRoom(roomID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.active[roomID]; ok {
		return false
	}
	q.active[roomID] = make(chan struct{})
	return true
}

// ReleaseRoom marks a room as no longer active.
func (q *MessageQueue) ReleaseRoom(roomID string) {
	q.mu.Lock()
	ch, ok := q.active[roomID]
	if ok {
		delete(q.active, roomID)
	}
	q.mu.Unlock()
	if ok && ch != nil {
		close(ch)
	}
}

// HasActiveRoom returns true if the given room is currently being processed.
func (q *MessageQueue) HasActiveRoom(roomID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.active[roomID]
	return ok
}

// acquireOrWait atomically acquires the room or waits for it to become free.
// This avoids the TOCTOU race between checking and acquiring.
func (q *MessageQueue) acquireOrWait(roomID string) {
	for {
		q.mu.Lock()
		ch, ok := q.active[roomID]
		if !ok {
			// Room is free — acquire it atomically within the same lock.
			q.active[roomID] = make(chan struct{})
			q.mu.Unlock()
			return
		}
		q.mu.Unlock()
		// Room is active — wait for it to be released, then retry.
		<-ch
	}
}
