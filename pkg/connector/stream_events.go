package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

// emitStreamEvent sends an AI SDK UIMessageChunk streaming event to the room.
func (oc *AIClient) emitStreamEvent(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	part map[string]any,
) {
	if portal == nil || portal.MXID == "" {
		return
	}
	if state == nil {
		return
	}
	if state != nil && state.suppressSend {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	state.sequenceNum++
	seq := state.sequenceNum

	content := map[string]any{
		"turn_id": state.turnID,
		"seq":     seq,
		"part":    part,
	}
	if state.initialEventID != "" {
		content["target_event"] = state.initialEventID.String()
		content["m.relates_to"] = map[string]any{
			"rel_type": RelReference,
			"event_id": state.initialEventID.String(),
		}
	}
	if state.agentID != "" {
		content["agent_id"] = state.agentID
	}

	eventContent := &event.Content{Raw: content}

	if _, err := intent.SendMessage(ctx, portal.MXID, StreamEventMessageType, eventContent, nil); err != nil {
		partType, _ := part["type"].(string)
		oc.log.Warn().Err(err).
			Str("part_type", partType).
			Int("seq", seq).
			Msg("Failed to emit stream event")
	}
}
