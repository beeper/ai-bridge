package connector

import (
	"context"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/agents"
)

// sendInitialStreamMessage sends the first message in a streaming session and returns its event ID
func (oc *AIClient) sendInitialStreamMessage(ctx context.Context, portal *bridgev2.Portal, content string, turnID string, replyTo id.EventID) id.EventID {
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	var relatesTo map[string]any
	if replyTo != "" {
		relatesTo = map[string]any{
			"m.in_reply_to": map[string]any{
				"event_id": replyTo.String(),
			},
		}
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype":      event.MsgText,
			"body":         content,
			"m.relates_to": relatesTo,
			BeeperAIKey: map[string]any{
				"turn_id": turnID,
				"status":  string(TurnStatusGenerating),
			},
		},
	}
	resp, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		oc.log.Error().Err(err).Msg("Failed to send initial streaming message")
		return ""
	}
	oc.log.Info().Stringer("event_id", resp.EventID).Str("turn_id", turnID).Msg("Initial streaming message sent")
	return resp.EventID
}

// sendFinalAssistantTurn sends an edit event with the complete assistant turn data.
// It processes response directives (reply tags, silent replies) before sending when in natural mode.
// Matches OpenClaw's directive processing behavior.
func (oc *AIClient) sendFinalAssistantTurn(ctx context.Context, portal *bridgev2.Portal, state *streamingState, meta *PortalMetadata) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	rawContent := state.accumulated.String()

	// Check response mode - raw mode skips directive processing
	responseMode := oc.getAgentResponseMode(meta)
	if responseMode == agents.ResponseModeRaw {
		// Raw mode: send content directly without directive processing
		rendered := format.RenderMarkdown(rawContent, true, true)
		oc.sendFinalAssistantTurnContent(ctx, portal, state, meta, intent, rendered, nil)
		return
	}

	// Natural mode: process directives (OpenClaw-style)
	directives := ParseResponseDirectives(rawContent, state.sourceEventID)

	// Handle silent replies - redact the streaming message
	if directives.IsSilent {
		oc.log.Debug().
			Str("turn_id", state.turnID).
			Str("initial_event_id", state.initialEventID.String()).
			Msg("Silent reply detected, redacting streaming message")

		// Redact the initial streaming message
		if state.initialEventID != "" {
			_, err := intent.SendMessage(ctx, portal.MXID, event.EventRedaction, &event.Content{
				Parsed: &event.RedactionEventContent{
					Redacts: state.initialEventID,
				},
			}, nil)
			if err != nil {
				oc.log.Warn().Err(err).Stringer("event_id", state.initialEventID).Msg("Failed to redact silent reply message")
			}
		}
		return
	}

	// Use cleaned content (directives stripped)
	cleanedContent := directives.Text
	rendered := format.RenderMarkdown(cleanedContent, true, true)

	// Build AI metadata following the new schema
	aiMetadata := map[string]any{
		"turn_id":       state.turnID,
		"model":         oc.effectiveModel(meta),
		"status":        string(TurnStatusCompleted),
		"finish_reason": state.finishReason,
		"timing": map[string]any{
			"started_at":     state.startedAtMs,
			"first_token_at": state.firstTokenAtMs,
			"completed_at":   state.completedAtMs,
		},
	}

	// Add agent_id if set
	if state.agentID != "" {
		aiMetadata["agent_id"] = state.agentID
	}

	if state.promptTokens > 0 || state.completionTokens > 0 || state.reasoningTokens > 0 {
		aiMetadata["usage"] = map[string]any{
			"prompt_tokens":     state.promptTokens,
			"completion_tokens": state.completionTokens,
			"reasoning_tokens":  state.reasoningTokens,
		}
	}

	// Include embedded thinking if present
	if state.reasoning.Len() > 0 {
		aiMetadata["thinking"] = map[string]any{
			"content":     state.reasoning.String(),
			"token_count": len(strings.Fields(state.reasoning.String())), // Approximate
		}
	}

	// Include tool call event IDs
	if len(state.toolCalls) > 0 {
		toolCallIDs := make([]string, 0, len(state.toolCalls))
		for _, tc := range state.toolCalls {
			if tc.CallEventID != "" {
				toolCallIDs = append(toolCallIDs, tc.CallEventID)
			}
		}
		if len(toolCallIDs) > 0 {
			aiMetadata["tool_calls"] = toolCallIDs
		}
	}

	// Build m.relates_to with replace relation
	relatesTo := map[string]any{
		"rel_type": RelReplace,
		"event_id": state.initialEventID.String(),
	}

	// Add reply relation if directive specifies one
	if directives.ReplyToEventID != "" {
		relatesTo["m.in_reply_to"] = map[string]any{
			"event_id": directives.ReplyToEventID.String(),
		}
	}

	// Generate link previews for URLs in the response
	linkPreviews := oc.generateOutboundLinkPreviews(ctx, cleanedContent, intent, portal)

	// Send edit event with m.replace relation and m.new_content
	eventRawContent := map[string]any{
		"msgtype":        event.MsgText,
		"body":           "* " + rendered.Body, // Fallback with edit marker
		"format":         rendered.Format,
		"formatted_body": "* " + rendered.FormattedBody,
		"m.new_content": map[string]any{
			"msgtype":        event.MsgText,
			"body":           rendered.Body,
			"format":         rendered.Format,
			"formatted_body": rendered.FormattedBody,
		},
		"m.relates_to":                  relatesTo,
		BeeperAIKey:                     aiMetadata,
		"com.beeper.dont_render_edited": true, // Don't show "edited" indicator for streaming updates
	}

	// Attach link previews if any were generated
	if len(linkPreviews) > 0 {
		eventRawContent["com.beeper.linkpreviews"] = PreviewsToMapSlice(linkPreviews)
	}

	eventContent := &event.Content{Raw: eventRawContent}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).Stringer("initial_event_id", state.initialEventID).Msg("Failed to send final assistant turn")
	} else {
		oc.log.Debug().
			Str("initial_event_id", state.initialEventID.String()).
			Str("turn_id", state.turnID).
			Bool("has_thinking", state.reasoning.Len() > 0).
			Int("tool_calls", len(state.toolCalls)).
			Bool("has_reply", directives.ReplyToEventID != "").
			Int("link_previews", len(linkPreviews)).
			Msg("Sent final assistant turn with metadata")
	}
}

// sendFinalAssistantTurnContent is a helper for raw mode that sends content without directive processing.
func (oc *AIClient) sendFinalAssistantTurnContent(ctx context.Context, portal *bridgev2.Portal, state *streamingState, meta *PortalMetadata, intent bridgev2.MatrixAPI, rendered event.MessageEventContent, replyToEventID *id.EventID) {
	// Build AI metadata
	aiMetadata := map[string]any{
		"turn_id":       state.turnID,
		"model":         oc.effectiveModel(meta),
		"status":        string(TurnStatusCompleted),
		"finish_reason": state.finishReason,
		"timing": map[string]any{
			"started_at":     state.startedAtMs,
			"first_token_at": state.firstTokenAtMs,
			"completed_at":   state.completedAtMs,
		},
	}

	if state.agentID != "" {
		aiMetadata["agent_id"] = state.agentID
	}

	if state.promptTokens > 0 || state.completionTokens > 0 || state.reasoningTokens > 0 {
		aiMetadata["usage"] = map[string]any{
			"prompt_tokens":     state.promptTokens,
			"completion_tokens": state.completionTokens,
			"reasoning_tokens":  state.reasoningTokens,
		}
	}

	if state.reasoning.Len() > 0 {
		aiMetadata["thinking"] = map[string]any{
			"content":     state.reasoning.String(),
			"token_count": len(strings.Fields(state.reasoning.String())),
		}
	}

	if len(state.toolCalls) > 0 {
		toolCallIDs := make([]string, 0, len(state.toolCalls))
		for _, tc := range state.toolCalls {
			if tc.CallEventID != "" {
				toolCallIDs = append(toolCallIDs, tc.CallEventID)
			}
		}
		if len(toolCallIDs) > 0 {
			aiMetadata["tool_calls"] = toolCallIDs
		}
	}

	relatesTo := map[string]any{
		"rel_type": RelReplace,
		"event_id": state.initialEventID.String(),
	}

	if replyToEventID != nil && *replyToEventID != "" {
		relatesTo["m.in_reply_to"] = map[string]any{
			"event_id": replyToEventID.String(),
		}
	}

	// Generate link previews for URLs in the response
	linkPreviews := oc.generateOutboundLinkPreviews(ctx, rendered.Body, intent, portal)

	rawContent2 := map[string]any{
		"msgtype":                       event.MsgText,
		"body":                          "* " + rendered.Body,
		"format":                        rendered.Format,
		"formatted_body":                "* " + rendered.FormattedBody,
		"m.new_content":                 map[string]any{"msgtype": event.MsgText, "body": rendered.Body, "format": rendered.Format, "formatted_body": rendered.FormattedBody},
		"m.relates_to":                  relatesTo,
		BeeperAIKey:                     aiMetadata,
		"com.beeper.dont_render_edited": true,
	}

	// Attach link previews if any were generated
	if len(linkPreviews) > 0 {
		rawContent2["com.beeper.linkpreviews"] = PreviewsToMapSlice(linkPreviews)
	}

	eventContent := &event.Content{Raw: rawContent2}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).Stringer("initial_event_id", state.initialEventID).Msg("Failed to send final assistant turn (raw mode)")
	} else {
		oc.log.Debug().
			Str("initial_event_id", state.initialEventID.String()).
			Str("turn_id", state.turnID).
			Str("mode", "raw").
			Int("link_previews", len(linkPreviews)).
			Msg("Sent final assistant turn (raw mode)")
	}
}

// generateOutboundLinkPreviews extracts URLs from AI response text, generates link previews, and uploads images to Matrix.
func (oc *AIClient) generateOutboundLinkPreviews(ctx context.Context, text string, intent bridgev2.MatrixAPI, portal *bridgev2.Portal) []*event.BeeperLinkPreview {
	config := oc.getLinkPreviewConfig()
	if !config.Enabled {
		return nil
	}

	urls := ExtractURLs(text, config.MaxURLsOutbound)
	if len(urls) == 0 {
		return nil
	}

	previewer := NewLinkPreviewer(config)
	fetchCtx, cancel := context.WithTimeout(ctx, config.FetchTimeout*time.Duration(len(urls)))
	defer cancel()

	previewsWithImages := previewer.FetchPreviews(fetchCtx, urls)

	// Upload images to Matrix and get final previews
	return UploadPreviewImages(ctx, previewsWithImages, intent, portal.MXID)
}

// getAgentResponseMode returns the response mode for the current agent.
// Defaults to ResponseModeNatural if not set.
// IsRawMode on the portal overrides all other settings (for playground rooms).
func (oc *AIClient) getAgentResponseMode(meta *PortalMetadata) agents.ResponseMode {
	// IsRawMode flag takes priority (set by playground command)
	if meta.IsRawMode {
		return agents.ResponseModeRaw
	}

	agentID := resolveAgentID(meta)

	if agentID != "" {
		store := NewAgentStoreAdapter(oc)
		if agent, err := store.GetAgentByID(context.Background(), agentID); err == nil && agent != nil {
			if agent.ResponseMode != "" {
				return agent.ResponseMode
			}
		}
	}

	// Default to natural mode (OpenClaw-style)
	return agents.ResponseModeNatural
}
