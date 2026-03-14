package ai

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"github.com/beeper/agentremote"
)

func TestOpenAIRemoteMessageAccessors(t *testing.T) {
	ts := time.Unix(123, 0)
	msg := &OpenAIRemoteMessage{
		PortalKey: networkid.PortalKey{ID: networkid.PortalID("portal")},
		ID:        networkid.MessageID("msg-1"),
		Sender:    bridgev2.EventSender{Sender: networkid.UserID("agent")},
		Timestamp: ts,
		Metadata:  &MessageMetadata{AssistantMessageMetadata: agentremote.AssistantMessageMetadata{CompletionID: "completion-1"}},
	}

	if got := msg.GetType(); got != bridgev2.RemoteEventMessage {
		t.Fatalf("expected remote message type, got %q", got)
	}
	if got := msg.GetPortalKey(); got != msg.PortalKey {
		t.Fatalf("expected portal key %#v, got %#v", msg.PortalKey, got)
	}
	if got := msg.GetSender(); got != msg.Sender {
		t.Fatalf("expected sender %#v, got %#v", msg.Sender, got)
	}
	if got := msg.GetID(); got != msg.ID {
		t.Fatalf("expected message id %q, got %q", msg.ID, got)
	}
	if got := msg.GetTimestamp(); !got.Equal(ts) {
		t.Fatalf("expected timestamp %v, got %v", ts, got)
	}
	var withOrder bridgev2.RemoteEventWithStreamOrder = msg
	if got := withOrder.GetStreamOrder(); got != ts.UnixMilli() {
		t.Fatalf("expected stream order to fall back to timestamp, got %d", got)
	}
	if got := msg.GetTransactionID(); got != networkid.TransactionID("completion-completion-1") {
		t.Fatalf("expected transaction id from completion id, got %q", got)
	}

	logger := zerolog.Nop()
	_ = msg.AddLogContext(logger.With())
}

func TestOpenAIRemoteMessageConvertMessage(t *testing.T) {
	testCases := []struct {
		name             string
		content          string
		formattedContent string
	}{
		{
			name:             "formatted content",
			content:          "hello world",
			formattedContent: "<strong>hello world</strong>",
		},
		{
			name:    "plain content",
			content: "plain text",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			meta := &MessageMetadata{
				AssistantMessageMetadata: agentremote.AssistantMessageMetadata{
					Model:        "gpt-test",
					CompletionID: "completion-2",
				},
			}
			msg := &OpenAIRemoteMessage{
				Content:          tc.content,
				FormattedContent: tc.formattedContent,
				Metadata:         meta,
			}

			converted, err := msg.ConvertMessage(context.Background(), nil, nil)
			if err != nil {
				t.Fatalf("expected conversion to succeed, got %v", err)
			}
			if converted == nil || len(converted.Parts) == 0 {
				t.Fatalf("expected converted message parts, got %#v", converted)
			}
			part := converted.Parts[0]
			if part.Type != event.EventMessage {
				t.Fatalf("expected first part type %q, got %q", event.EventMessage, part.Type)
			}
			if part.Content == nil {
				t.Fatalf("expected first part content")
			}
			if part.Content.Body != tc.content {
				t.Fatalf("expected body %q, got %q", tc.content, part.Content.Body)
			}
			if tc.formattedContent != "" {
				if part.Content.FormattedBody != tc.formattedContent {
					t.Fatalf("expected formatted body %q, got %q", tc.formattedContent, part.Content.FormattedBody)
				}
			} else if part.Content.FormattedBody != "" {
				t.Fatalf("expected empty formatted body, got %q", part.Content.FormattedBody)
			}
			if meta.Body != tc.content {
				t.Fatalf("expected metadata body to be backfilled from content, got %q", meta.Body)
			}
		})
	}
}
