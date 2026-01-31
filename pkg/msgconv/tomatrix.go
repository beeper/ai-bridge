package msgconv

import (
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"github.com/beeper/ai-bridge/pkg/aiid"
)

// ConvertAIResponseToMatrix converts an AI response to a Matrix message
func ConvertAIResponseToMatrix(content string, metadata *aiid.MessageMetadata, portalMeta *aiid.PortalMetadata) *bridgev2.ConvertedMessage {
	msgContent := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    content,
	}

	// Build Extra map with AI-specific metadata
	extra := map[string]any{}
	if metadata != nil {
		if metadata.CompletionID != "" {
			extra["com.beeper.ai.completion_id"] = metadata.CompletionID
		}
		if metadata.FinishReason != "" {
			extra["com.beeper.ai.finish_reason"] = metadata.FinishReason
		}
		if metadata.PromptTokens > 0 {
			extra["com.beeper.ai.prompt_tokens"] = metadata.PromptTokens
		}
		if metadata.CompletionTokens > 0 {
			extra["com.beeper.ai.completion_tokens"] = metadata.CompletionTokens
		}
		if metadata.Model != "" {
			extra["com.beeper.ai.model"] = metadata.Model
		}
		if metadata.ReasoningTokens > 0 {
			extra["com.beeper.ai.reasoning_tokens"] = metadata.ReasoningTokens
		}
		if metadata.HasToolCalls {
			extra["com.beeper.ai.has_tool_calls"] = metadata.HasToolCalls
		}

		// Update body in metadata if empty
		if metadata.Body == "" {
			metadata.Body = content
		}
	}

	// Get model from portal metadata as fallback
	if _, hasModel := extra["com.beeper.ai.model"]; !hasModel {
		if portalMeta != nil && portalMeta.Model != "" {
			extra["com.beeper.ai.model"] = portalMeta.Model
		}
	}

	part := &bridgev2.ConvertedMessagePart{
		ID:         networkid.PartID("0"),
		Type:       event.EventMessage,
		Content:    msgContent,
		Extra:      extra,
		DBMetadata: metadata,
	}

	return &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{part},
	}
}

// BuildStreamEditContent creates the content for a streaming edit message
func BuildStreamEditContent(body, reasoning, model, finishReason string) map[string]any {
	// Build AI metadata for rich UI rendering
	aiMetadata := map[string]any{
		"model":         model,
		"finish_reason": finishReason,
	}

	// Include reasoning/thinking if present
	if reasoning != "" {
		aiMetadata["thinking"] = reasoning
	}

	// Build edit event content with m.replace relation and m.new_content
	return map[string]any{
		"msgtype": "m.text",
		"body":    "* " + body, // Fallback with edit marker
		"m.new_content": map[string]any{
			"msgtype": "m.text",
			"body":    body,
		},
		"com.beeper.ai":                 aiMetadata,
		"com.beeper.dont_render_edited": true, // Don't show "edited" indicator for streaming updates
	}
}
