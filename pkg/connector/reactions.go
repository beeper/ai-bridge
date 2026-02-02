package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// sendReaction sends a reaction emoji to a Matrix event.
func (oc *AIClient) sendReaction(ctx context.Context, portal *bridgev2.Portal, targetEventID id.EventID, emoji string) {
	if portal == nil || portal.MXID == "" || targetEventID == "" || emoji == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"m.relates_to": map[string]any{
				"rel_type": "m.annotation",
				"event_id": targetEventID.String(),
				"key":      emoji,
			},
		},
	}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventReaction, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).
			Stringer("target_event", targetEventID).
			Str("emoji", emoji).
			Msg("Failed to send reaction")
	} else {
		oc.log.Debug().
			Stringer("target_event", targetEventID).
			Str("emoji", emoji).
			Msg("Sent reaction")
	}
}
