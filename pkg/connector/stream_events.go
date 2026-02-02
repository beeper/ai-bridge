package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

// emitStreamDelta sends a streaming delta event to the room
func (oc *AIClient) emitStreamDelta(ctx context.Context, portal *bridgev2.Portal, state *streamingState, contentType StreamContentType, delta string, extra map[string]any) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"turn_id":      state.turnID,
			"target_event": state.initialEventID.String(),
			"content_type": string(contentType),
			"delta":        delta,
			"seq":          state.sequenceNum,
			"m.relates_to": map[string]any{
				"rel_type": RelReference,
				"event_id": state.initialEventID.String(),
			},
		},
	}

	// Add agent_id if set
	if state.agentID != "" {
		eventContent.Raw["agent_id"] = state.agentID
	}

	// Merge extra fields
	for k, v := range extra {
		if _, exists := eventContent.Raw[k]; !exists {
			eventContent.Raw[k] = v
		}
	}

	if _, err := intent.SendMessage(ctx, portal.MXID, StreamDeltaEventType, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).
			Stringer("target_event", state.initialEventID).
			Str("content_type", string(contentType)).
			Int("seq", state.sequenceNum).
			Msg("Failed to emit stream delta")
	}
}

// emitGenerationStatus sends a generation status update event
func (oc *AIClient) emitGenerationStatus(ctx context.Context, portal *bridgev2.Portal, state *streamingState, statusType string, message string, details *GenerationDetails) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	content := map[string]any{
		"turn_id":        state.turnID,
		"status":         statusType,
		"status_message": message,
	}

	if state.initialEventID != "" {
		content["target_event"] = state.initialEventID.String()
	}

	if state.agentID != "" {
		content["agent_id"] = state.agentID
	}

	if details != nil {
		detailsMap := map[string]any{}
		if details.CurrentTool != "" {
			detailsMap["current_tool"] = details.CurrentTool
		}
		if details.CallID != "" {
			detailsMap["call_id"] = details.CallID
		}
		if details.ToolsCompleted > 0 || details.ToolsTotal > 0 {
			detailsMap["tools_completed"] = details.ToolsCompleted
			detailsMap["tools_total"] = details.ToolsTotal
		}
		if len(detailsMap) > 0 {
			content["details"] = detailsMap
		}
	}

	eventContent := &event.Content{Raw: content}

	if _, err := intent.SendMessage(ctx, portal.MXID, GenerationStatusEventType, eventContent, nil); err != nil {
		oc.log.Debug().Err(err).Str("status", statusType).Msg("Failed to emit generation status")
	}
}
