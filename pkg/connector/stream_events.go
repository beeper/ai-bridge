package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

type StreamEventSource string

const (
	StreamEventSourceResponses       StreamEventSource = "responses"
	StreamEventSourceChatCompletions StreamEventSource = "chat_completions"
	StreamEventSourceInternal        StreamEventSource = "internal"
)

type StreamEventRawPayload struct {
	Type string
	JSON string
}

// emitStreamEvent sends a unified streaming event to the room.
func (oc *AIClient) emitStreamEvent(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	source StreamEventSource,
	kind string,
	data map[string]any,
	raw *StreamEventRawPayload,
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
		"source":  string(source),
		"kind":    kind,
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
	if data != nil {
		content["data"] = data
	}
	if raw != nil {
		content["raw"] = map[string]any{
			"type": raw.Type,
			"json": raw.JSON,
		}
	}

	eventContent := &event.Content{Raw: content}

	if _, err := intent.SendMessage(ctx, portal.MXID, StreamEventMessageType, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).
			Str("kind", kind).
			Int("seq", seq).
			Msg("Failed to emit stream event")
	}
}

func (oc *AIClient) emitStatusEvent(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	statusType string,
	message string,
	details *GenerationDetails,
) {
	data := map[string]any{
		"status": statusType,
	}
	if message != "" {
		data["message"] = message
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
			data["details"] = detailsMap
		}
	}
	oc.emitStreamEvent(ctx, portal, state, StreamEventSourceInternal, "status", data, nil)
}
