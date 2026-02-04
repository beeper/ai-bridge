package connector

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type matrixEphemeralSender interface {
	SendEphemeral(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, txnID string) (*mautrix.RespSendEvent, error)
}

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
	ephemeralSender, ok := intent.(matrixEphemeralSender)
	if !ok {
		partType, _ := part["type"].(string)
		oc.log.Warn().
			Str("part_type", partType).
			Int("seq", seq).
			Msg("Matrix intent does not support ephemeral events; dropping stream event")
		return
	}

	txnID := buildStreamEventTxnID(state.turnID, seq)
	if _, err := ephemeralSender.SendEphemeral(ctx, portal.MXID, StreamEventMessageType, eventContent, txnID); err != nil {
		partType, _ := part["type"].(string)
		oc.log.Warn().Err(err).
			Str("part_type", partType).
			Int("seq", seq).
			Msg("Failed to emit stream event")
	}
}

func buildStreamEventTxnID(turnID string, seq int) string {
	if turnID == "" {
		return fmt.Sprintf("ai_stream_%d", seq)
	}
	return fmt.Sprintf("ai_stream_%s_%d", turnID, seq)
}
