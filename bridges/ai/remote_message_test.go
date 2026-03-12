package ai

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func TestOpenAIRemoteMessageAccessors(t *testing.T) {
	ts := time.Unix(123, 0)
	msg := &OpenAIRemoteMessage{
		PortalKey: networkid.PortalKey{ID: networkid.PortalID("portal")},
		ID:        networkid.MessageID("msg-1"),
		Sender:    bridgev2.EventSender{Sender: networkid.UserID("agent")},
		Timestamp: ts,
		Metadata:  &MessageMetadata{CompletionID: "completion-1"},
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
	meta := &MessageMetadata{
		Model:        "gpt-test",
		CompletionID: "completion-2",
	}
	msg := &OpenAIRemoteMessage{
		Content:          "hello world",
		FormattedContent: "<strong>hello world</strong>",
		Metadata:         meta,
	}

	converted, err := msg.ConvertMessage(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if converted == nil || len(converted.Parts) == 0 {
		t.Fatalf("expected converted message parts, got %#v", converted)
	}
	if meta.Body != "hello world" {
		t.Fatalf("expected metadata body to be backfilled from content, got %q", meta.Body)
	}
}
