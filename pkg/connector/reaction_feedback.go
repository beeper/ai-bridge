package connector

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// ReactionFeedback represents a user reaction to an AI message.
// Similar to OpenClaw's system events, these are queued and drained
// when building the next prompt.
type ReactionFeedback struct {
	Emoji     string    // The emoji used (e.g., "👍", "👎")
	Timestamp time.Time // When the reaction was added
	Sender    string    // Who sent the reaction (display name or user ID)
	MessageID string    // Which message was reacted to
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
// Matches OpenClaw's system event format: "System: [timestamp] Matrix reaction added: :emoji: by user"
func FormatReactionFeedback(feedback []ReactionFeedback) string {
	if len(feedback) == 0 {
		return ""
	}

	var lines []string
	for _, f := range feedback {
		ts := f.Timestamp.Format("2006-01-02 15:04:05")
		if f.Action == "removed" {
			lines = append(lines, fmt.Sprintf("System: [%s] Matrix reaction removed: %s by %s", ts, f.Emoji, f.Sender))
		} else {
			lines = append(lines, fmt.Sprintf("System: [%s] Matrix reaction added: %s by %s", ts, f.Emoji, f.Sender))
		}
	}
	return strings.Join(lines, "\n")
}

// registerReactionHandler registers a handler for Matrix reaction events.
func (oc *OpenAIConnector) registerReactionHandler() {
	matrixConnector, ok := oc.br.Matrix.(*matrix.Connector)
	if !ok {
		oc.br.Log.Warn().Msg("Cannot register reaction handler: Matrix connector type assertion failed")
		return
	}

	// Handle reaction events
	matrixConnector.EventProcessor.On(event.EventReaction, func(ctx context.Context, evt *event.Event) {
		oc.handleReactionEvent(ctx, evt)
	})

	// Handle redactions (which may include reaction removals)
	matrixConnector.EventProcessor.On(event.EventRedaction, func(ctx context.Context, evt *event.Event) {
		oc.handleRedactionEvent(ctx, evt)
	})

	oc.br.Log.Info().Msg("Registered reaction feedback handler")
}

// handleReactionEvent processes incoming Matrix reactions.
func (oc *OpenAIConnector) handleReactionEvent(ctx context.Context, evt *event.Event) {
	log := oc.br.Log.With().
		Str("component", "reaction_handler").
		Str("room_id", evt.RoomID.String()).
		Str("sender", evt.Sender.String()).
		Logger()

	// Parse reaction content
	content, ok := evt.Content.Parsed.(*event.ReactionEventContent)
	if !ok {
		log.Debug().Msg("Failed to parse reaction content")
		return
	}

	// Get the target event ID
	targetEventID := content.RelatesTo.EventID
	if targetEventID == "" {
		return
	}

	// Check if this room is a portal we manage
	portal, err := oc.getPortalByRoomID(ctx, evt.RoomID)
	if err != nil || portal == nil {
		// Not one of our portals, ignore
		return
	}

	// Skip reactions from the bot itself
	if oc.isBotUser(evt.Sender) {
		return
	}

	// Get sender display name - use localpart as fallback
	senderName := evt.Sender.Localpart()

	// Enqueue the reaction feedback
	// We queue all reactions in AI rooms - the AI will naturally understand
	// the context since it knows which messages it sent
	feedback := ReactionFeedback{
		Emoji:     content.RelatesTo.Key,
		Timestamp: time.Now(),
		Sender:    senderName,
		MessageID: targetEventID.String(),
		Action:    "added",
	}

	EnqueueReactionFeedback(evt.RoomID, feedback)
	log.Debug().
		Str("emoji", feedback.Emoji).
		Str("target", feedback.MessageID).
		Msg("Enqueued reaction feedback")
}

// isBotUser checks if a user ID belongs to the bridge bot or a ghost.
func (oc *OpenAIConnector) isBotUser(userID id.UserID) bool {
	// Check if it's the main bot
	if oc.br.Bot != nil && oc.br.Bot.GetMXID() == userID {
		return true
	}
	// Check if it looks like one of our ghost users (they have a specific pattern)
	localpart := userID.Localpart()
	return strings.HasPrefix(localpart, "aibot_") || strings.HasPrefix(localpart, "openaibot_")
}

// handleRedactionEvent processes redactions that might be reaction removals.
func (oc *OpenAIConnector) handleRedactionEvent(ctx context.Context, evt *event.Event) {
	// For now, we don't track reaction removals as it requires
	// keeping track of which events were reactions.
	// This could be enhanced later if needed.
}

// getPortalByRoomID gets a portal by its Matrix room ID.
func (oc *OpenAIConnector) getPortalByRoomID(ctx context.Context, roomID id.RoomID) (*bridgev2.Portal, error) {
	// Query all portals and find the one matching this room ID
	allPortals, err := oc.br.DB.Portal.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, dbPortal := range allPortals {
		if dbPortal.MXID == roomID {
			return oc.br.GetPortalByKey(ctx, dbPortal.PortalKey)
		}
	}

	return nil, nil
}
