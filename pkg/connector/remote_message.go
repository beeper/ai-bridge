package connector

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	_ bridgev2.RemoteMessage                  = (*OpenAIRemoteMessage)(nil)
	_ bridgev2.RemoteEventWithTimestamp       = (*OpenAIRemoteMessage)(nil)
	_ bridgev2.RemoteMessageWithTransactionID = (*OpenAIRemoteMessage)(nil)
)

// OpenAIRemoteMessage represents a GPT answer that should be bridged to Matrix.
type OpenAIRemoteMessage struct {
	PortalKey networkid.PortalKey
	ID        networkid.MessageID
	Sender    bridgev2.EventSender
	Content   string
	Timestamp time.Time
	Metadata  *MessageMetadata

	// New schema fields
	FormattedContent string     // HTML formatted content
	ReplyToEventID   id.EventID // For m.relates_to threading
	ToolCallEventIDs []string   // References to tool call events
	ImageEventIDs    []string   // References to generated image events
}

func (m *OpenAIRemoteMessage) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventMessage
}

func (m *OpenAIRemoteMessage) GetPortalKey() networkid.PortalKey {
	return m.PortalKey
}

func (m *OpenAIRemoteMessage) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.Str("openai_message_id", string(m.ID))
}

func (m *OpenAIRemoteMessage) GetSender() bridgev2.EventSender {
	return m.Sender
}

func (m *OpenAIRemoteMessage) GetID() networkid.MessageID {
	return m.ID
}

func (m *OpenAIRemoteMessage) GetTimestamp() time.Time {
	if m.Timestamp.IsZero() {
		return time.Now()
	}
	return m.Timestamp
}

// GetTransactionID implements RemoteMessageWithTransactionID
func (m *OpenAIRemoteMessage) GetTransactionID() networkid.TransactionID {
	// Use completion ID as transaction ID for deduplication
	if m.Metadata != nil && m.Metadata.CompletionID != "" {
		return networkid.TransactionID("completion-" + m.Metadata.CompletionID)
	}
	return ""
}

func (m *OpenAIRemoteMessage) ConvertMessage(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI) (*bridgev2.ConvertedMessage, error) {
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    m.Content,
	}

	// Add formatted content if available
	if m.FormattedContent != "" {
		content.Format = event.FormatHTML
		content.FormattedBody = m.FormattedContent
	}

	if m.Metadata != nil && m.Metadata.Body == "" {
		m.Metadata.Body = m.Content
	}

	// Build the new com.beeper.ai nested structure
	extra := map[string]any{}

	// Get model from metadata or portal fallback
	model := ""
	if m.Metadata != nil && m.Metadata.Model != "" {
		model = m.Metadata.Model
	} else if portalMeta, ok := portal.Metadata.(*PortalMetadata); ok && portalMeta.Model != "" {
		model = portalMeta.Model
	}

	// Build the com.beeper.ai content block
	aiContent := map[string]any{}

	if m.Metadata != nil {
		// Core fields
		if m.Metadata.TurnID != "" {
			aiContent["turn_id"] = m.Metadata.TurnID
		}
		if m.Metadata.AgentID != "" {
			aiContent["agent_id"] = m.Metadata.AgentID
		}
		if model != "" {
			aiContent["model"] = model
		}

		// Status and completion info
		aiContent["status"] = TurnStatusCompleted
		if m.Metadata.FinishReason != "" {
			aiContent["finish_reason"] = m.Metadata.FinishReason
		}

		// Embedded thinking
		if m.Metadata.ThinkingContent != "" {
			thinking := map[string]any{
				"content": m.Metadata.ThinkingContent,
			}
			if m.Metadata.ThinkingTokenCount > 0 {
				thinking["token_count"] = m.Metadata.ThinkingTokenCount
			}
			aiContent["thinking"] = thinking
		}

		// Usage info
		usage := map[string]any{}
		if m.Metadata.PromptTokens > 0 {
			usage["prompt_tokens"] = m.Metadata.PromptTokens
		}
		if m.Metadata.CompletionTokens > 0 {
			usage["completion_tokens"] = m.Metadata.CompletionTokens
		}
		if m.Metadata.ReasoningTokens > 0 {
			usage["reasoning_tokens"] = m.Metadata.ReasoningTokens
		}
		if len(usage) > 0 {
			aiContent["usage"] = usage
		}

		// Tool call references
		if len(m.ToolCallEventIDs) > 0 {
			aiContent["tool_calls"] = m.ToolCallEventIDs
		} else if m.Metadata.HasToolCalls && len(m.Metadata.ToolCalls) > 0 {
			// Build tool call IDs from metadata
			toolCallIDs := make([]string, 0, len(m.Metadata.ToolCalls))
			for _, tc := range m.Metadata.ToolCalls {
				if tc.CallEventID != "" {
					toolCallIDs = append(toolCallIDs, tc.CallEventID)
				}
			}
			if len(toolCallIDs) > 0 {
				aiContent["tool_calls"] = toolCallIDs
			}
		}

		// Image references
		if len(m.ImageEventIDs) > 0 {
			aiContent["images"] = m.ImageEventIDs
		}

		// Timing info
		timing := map[string]any{}
		if m.Metadata.StartedAtMs > 0 {
			timing["started_at"] = m.Metadata.StartedAtMs
		}
		if m.Metadata.FirstTokenAtMs > 0 {
			timing["first_token_at"] = m.Metadata.FirstTokenAtMs
		}
		if m.Metadata.CompletedAtMs > 0 {
			timing["completed_at"] = m.Metadata.CompletedAtMs
		}
		if len(timing) > 0 {
			aiContent["timing"] = timing
		}

		// Legacy fields for backwards compatibility
		if m.Metadata.CompletionID != "" {
			aiContent["completion_id"] = m.Metadata.CompletionID
		}
	}

	// Only add the block if we have content
	if len(aiContent) > 0 {
		extra["com.beeper.ai"] = aiContent
	}

	// Build m.relates_to for threading if we have a reply target
	if m.ReplyToEventID != "" {
		extra["m.relates_to"] = map[string]any{
			"rel_type":        "m.thread",
			"event_id":        m.ReplyToEventID.String(),
			"is_falling_back": true,
			"m.in_reply_to": map[string]any{
				"event_id": m.ReplyToEventID.String(),
			},
		}
	}

	part := &bridgev2.ConvertedMessagePart{
		ID:         networkid.PartID("0"),
		Type:       event.EventMessage,
		Content:    content,
		Extra:      extra,
		DBMetadata: m.Metadata,
	}
	return &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{part},
	}, nil
}
