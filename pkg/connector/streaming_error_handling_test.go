package connector

import (
	"errors"
	"testing"

	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func TestStreamingStateHasInitialMessageTarget(t *testing.T) {
	t.Run("event-id", func(t *testing.T) {
		state := &streamingState{initialEventID: id.EventID("$evt")}
		if !state.hasInitialMessageTarget() {
			t.Fatalf("expected event-id target to be valid")
		}
	})

	t.Run("network-message-id", func(t *testing.T) {
		state := &streamingState{networkMessageID: networkid.MessageID("msg-1")}
		if !state.hasInitialMessageTarget() {
			t.Fatalf("expected network-message-id target to be valid")
		}
	})

	t.Run("none", func(t *testing.T) {
		state := &streamingState{}
		if state.hasInitialMessageTarget() {
			t.Fatalf("expected empty state to have no target")
		}
	})
}

func TestStreamFailureErrorUsesAnyMessageTarget(t *testing.T) {
	testErr := errors.New("boom")

	t.Run("with-network-message-id", func(t *testing.T) {
		err := streamFailureError(&streamingState{networkMessageID: networkid.MessageID("msg-1")}, testErr)
		var nf *NonFallbackError
		if !errors.As(err, &nf) {
			t.Fatalf("expected NonFallbackError, got %T", err)
		}
	})

	t.Run("without-target", func(t *testing.T) {
		err := streamFailureError(&streamingState{}, testErr)
		var pf *PreDeltaError
		if !errors.As(err, &pf) {
			t.Fatalf("expected PreDeltaError, got %T", err)
		}
	})
}
