package codex

import (
	"testing"
	"time"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/pkg/shared/streamui"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/id"
)

func TestCodex_StreamChunks_BasicOrderingAndSeq(t *testing.T) {
	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_local_1")
	attachTestTurn(state, portal)
	state.turn.Stream().Metadata(map[string]any{"model": "gpt-5.1-codex"})
	state.turn.Stream().StepStart()
	state.turn.Stream().TextDelta("hi")
	state.turn.End("completed")

	uiMessage := streamui.SnapshotCanonicalUIMessage(state.turn.UIState())
	gotParts := agentremote.NormalizeUIParts(uiMessage["parts"])
	if len(gotParts) < 5 {
		t.Fatalf("expected >=5 parts, got %d", len(gotParts))
	}
	if gotParts[0]["type"] != "start" {
		t.Fatalf("expected first part type=start, got %#v", gotParts[0]["type"])
	}
	if gotParts[1]["type"] != "start-step" {
		t.Fatalf("expected second part type=start-step, got %#v", gotParts[1]["type"])
	}
	// text-start then text-delta should be present before finish.
	seenTextStart := false
	seenTextDelta := false
	seenFinish := false
	for _, p := range gotParts {
		switch p["type"] {
		case "text-start":
			seenTextStart = true
		case "text-delta":
			seenTextDelta = true
		case "finish":
			seenFinish = true
		}
	}
	if !seenTextStart || !seenTextDelta {
		t.Fatalf("expected text-start and text-delta, got parts=%v", gotParts)
	}
	if !seenFinish {
		t.Fatalf("expected finish part, got parts=%v", gotParts)
	}
}

func TestCodexStreamEventTimestampPrefersStartedAndCompleted(t *testing.T) {
	state := &streamingState{
		startedAtMs:   time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC).UnixMilli(),
		completedAtMs: time.Date(2026, time.March, 12, 10, 0, 5, 0, time.UTC).UnixMilli(),
	}
	if got := codexStreamEventTimestamp(state, false); got.UnixMilli() != state.startedAtMs {
		t.Fatalf("expected startedAtMs timestamp, got %d", got.UnixMilli())
	}
	if got := codexStreamEventTimestamp(state, true); got.UnixMilli() != state.completedAtMs {
		t.Fatalf("expected completedAtMs timestamp, got %d", got.UnixMilli())
	}
}

func TestCodexNextLiveStreamOrderMonotonic(t *testing.T) {
	state := &streamingState{}
	ts := time.UnixMilli(1_700_000_000_000)
	first := codexNextLiveStreamOrder(state, ts)
	second := codexNextLiveStreamOrder(state, ts)
	if second <= first {
		t.Fatalf("expected monotonic stream order, got %d then %d", first, second)
	}
}
