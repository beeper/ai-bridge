package ai

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

func TestProcessStreamingTextDeltaEmitsPlainVisibleTextWithoutDirectives(t *testing.T) {
	oc := &AIClient{}
	state := newTestStreamingStateWithTurn()
	state.turn.SetSuppressSend(true)

	roundDelta, err := oc.processStreamingTextDelta(
		context.Background(),
		zerolog.Nop(),
		nil,
		state,
		nil,
		nil,
		false,
		"hello",
		"stream failed",
		"stream failed",
	)
	if err != nil {
		t.Fatalf("processStreamingTextDelta returned error: %v", err)
	}
	if roundDelta != "hello" {
		t.Fatalf("expected round delta hello, got %q", roundDelta)
	}
	if got := visibleStreamingText(state); got != "hello" {
		t.Fatalf("expected visible text hello, got %q", got)
	}
}
