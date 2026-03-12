package agentremote

import (
	"testing"
	"time"
)

func TestRemoteMessageGetStreamOrderUsesExplicitValue(t *testing.T) {
	msg := &RemoteMessage{
		Timestamp:   time.UnixMilli(1_000),
		StreamOrder: 42,
	}
	if got := msg.GetStreamOrder(); got != 42 {
		t.Fatalf("expected explicit stream order 42, got %d", got)
	}
}

func TestRemoteEditGetStreamOrderUsesExplicitValue(t *testing.T) {
	edit := &RemoteEdit{
		Timestamp:   time.UnixMilli(1_000),
		StreamOrder: 84,
	}
	if got := edit.GetStreamOrder(); got != 84 {
		t.Fatalf("expected explicit stream order 84, got %d", got)
	}
}
