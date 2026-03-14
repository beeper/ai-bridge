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

	uiState := state.turn.UIState()
	if uiState == nil || !uiState.UIStarted || !uiState.UIFinished {
		t.Fatalf("expected turn UI state to be started and finished, got %#v", uiState)
	}
	uiMessage := streamui.SnapshotCanonicalUIMessage(uiState)
	gotParts := agentremote.NormalizeUIParts(uiMessage["parts"])
	if len(gotParts) == 0 {
		t.Fatal("expected canonical UI parts")
	}
	if gotParts[0]["type"] != "text" {
		t.Fatalf("expected canonical text part, got %#v", gotParts[0])
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
