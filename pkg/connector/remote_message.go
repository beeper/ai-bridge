package connector

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
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
	if m.Metadata != nil && m.Metadata.Body == "" {
		m.Metadata.Body = m.Content
	}

	// Build Extra map with AI-specific metadata
	extra := map[string]any{}
	addIfNotEmpty := func(key string, val any) {
		switch v := val.(type) {
		case string:
			if v != "" {
				extra[key] = v
			}
		case int64:
			if v > 0 {
				extra[key] = v
			}
		case bool:
			if v {
				extra[key] = v
			}
		}
	}

	if m.Metadata != nil {
		addIfNotEmpty("com.beeper.ai.completion_id", m.Metadata.CompletionID)
		addIfNotEmpty("com.beeper.ai.finish_reason", m.Metadata.FinishReason)
		addIfNotEmpty("com.beeper.ai.prompt_tokens", m.Metadata.PromptTokens)
		addIfNotEmpty("com.beeper.ai.completion_tokens", m.Metadata.CompletionTokens)
		addIfNotEmpty("com.beeper.ai.model", m.Metadata.Model)
		addIfNotEmpty("com.beeper.ai.reasoning_tokens", m.Metadata.ReasoningTokens)
		addIfNotEmpty("com.beeper.ai.has_tool_calls", m.Metadata.HasToolCalls)
	}

	// Get model from portal metadata as fallback
	if _, hasModel := extra["com.beeper.ai.model"]; !hasModel {
		if portalMeta, ok := portal.Metadata.(*PortalMetadata); ok && portalMeta.Model != "" {
			extra["com.beeper.ai.model"] = portalMeta.Model
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
