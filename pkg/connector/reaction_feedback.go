package connector

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"
)

// ReactionFeedback represents a user reaction to an AI message.
// Similar to OpenClaw's system events, these are queued and drained
// when building the next prompt.
type ReactionFeedback struct {
	Emoji     string    // The emoji used (e.g., "👍", "👎")
	Timestamp time.Time // When the reaction was added
	Sender    string    // Who sent the reaction (display name or user ID)
	MessageID string    // Which message was reacted to (event ID or timestamp)
	RoomName  string    // Room/channel name for context
	Action    string    // "added" or "removed"
}

// ReactionQueue holds reaction feedback for a room.
type ReactionQueue struct {
	mu       sync.Mutex
	feedback []ReactionFeedback
	maxSize  int
	lastText string // For deduplication like OpenClaw
}

// reactionQueues stores per-room reaction feedback queues.
var (
	reactionQueues   = make(map[id.RoomID]*ReactionQueue)
	reactionQueuesMu sync.Mutex
)

const maxReactionFeedback = 10 // Keep last N reactions per room

// getReactionQueue returns or creates a reaction queue for a room.
func getReactionQueue(roomID id.RoomID) *ReactionQueue {
	reactionQueuesMu.Lock()
	defer reactionQueuesMu.Unlock()

	q, ok := reactionQueues[roomID]
	if !ok {
		q = &ReactionQueue{
			feedback: make([]ReactionFeedback, 0, maxReactionFeedback),
			maxSize:  maxReactionFeedback,
		}
		reactionQueues[roomID] = q
	}
	return q
}

// AddReaction adds a reaction feedback to the queue.
// Skips consecutive duplicates like OpenClaw does.
func (q *ReactionQueue) AddReaction(feedback ReactionFeedback) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Build text key for deduplication
	textKey := fmt.Sprintf("%s:%s:%s:%s", feedback.Action, feedback.Emoji, feedback.Sender, feedback.MessageID)
	if q.lastText == textKey {
		return // Skip consecutive duplicate
	}
	q.lastText = textKey

	q.feedback = append(q.feedback, feedback)
	if len(q.feedback) > q.maxSize {
		q.feedback = q.feedback[1:] // Remove oldest
	}
}

// DrainFeedback returns all queued feedback and clears the queue.
func (q *ReactionQueue) DrainFeedback() []ReactionFeedback {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.feedback) == 0 {
		return nil
	}

	result := make([]ReactionFeedback, len(q.feedback))
	copy(result, q.feedback)
	q.feedback = q.feedback[:0]
	q.lastText = "" // Reset deduplication state
	return result
}

// PeekFeedback returns queued feedback without clearing.
func (q *ReactionQueue) PeekFeedback() []ReactionFeedback {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.feedback) == 0 {
		return nil
	}

	result := make([]ReactionFeedback, len(q.feedback))
	copy(result, q.feedback)
	return result
}

// HasFeedback returns true if there's pending feedback.
func (q *ReactionQueue) HasFeedback() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.feedback) > 0
}

// EnqueueReactionFeedback adds reaction feedback for a room.
func EnqueueReactionFeedback(roomID id.RoomID, feedback ReactionFeedback) {
	q := getReactionQueue(roomID)
	q.AddReaction(feedback)
}

// DrainReactionFeedback returns and clears all reaction feedback for a room.
func DrainReactionFeedback(roomID id.RoomID) []ReactionFeedback {
	q := getReactionQueue(roomID)
	return q.DrainFeedback()
}

// FormatReactionFeedback formats reaction feedback as context for the AI.
// Matches OpenClaw's system event format: "System: [timestamp] Beeper/Matrix reaction added: :emoji: by user in #room msg ts"
func FormatReactionFeedback(feedback []ReactionFeedback) string {
	if len(feedback) == 0 {
		return ""
	}

	var lines []string
	for _, f := range feedback {
		ts := f.Timestamp.Format("2006-01-02 15:04:05")
		roomLabel := f.RoomName
		if roomLabel == "" {
			roomLabel = "chat"
		}
		if f.Action == "removed" {
			lines = append(lines, fmt.Sprintf("System: [%s] Beeper/Matrix reaction removed: %s by %s in %s msg %s", ts, f.Emoji, f.Sender, roomLabel, f.MessageID))
		} else {
			lines = append(lines, fmt.Sprintf("System: [%s] Beeper/Matrix reaction added: %s by %s in %s msg %s", ts, f.Emoji, f.Sender, roomLabel, f.MessageID))
		}
	}
	return strings.Join(lines, "\n")
}
