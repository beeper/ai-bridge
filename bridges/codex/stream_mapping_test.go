package codex

import (
	"context"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/pkg/shared/streamui"
	bridgesdk "github.com/beeper/agentremote/sdk"
)

func newHookableStreamingState(turnID string) *streamingState {
	return &streamingState{
		turnID:           turnID,
		initialEventID:   id.EventID("$event"),
		networkMessageID: networkid.MessageID("codex:test"),
	}
}

func attachTestTurn(state *streamingState, portal *bridgev2.Portal) {
	if state == nil {
		return
	}
	conv := bridgesdk.NewConversation(context.Background(), nil, portal, bridgev2.EventSender{}, &bridgesdk.Config{}, nil)
	turn := conv.StartTurn(context.Background(), nil, nil)
	turn.SetID(state.turnID)
	state.turn = turn
}

func uiPartTypes(state *streamingState) []string {
	if state == nil || state.turn == nil || state.turn.UIState() == nil {
		return nil
	}
	uiMessage := streamui.SnapshotCanonicalUIMessage(state.turn.UIState())
	parts := agentremote.NormalizeUIParts(uiMessage["parts"])
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if typ, _ := part["type"].(string); typ != "" {
			out = append(out, typ)
		}
	}
	return out
}

func TestCodex_Mapping_AgentMessageDelta_EmitsTextStartThenDelta(t *testing.T) {
	cc := &CodexClient{}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_1")
	attachTestTurn(state, portal)
	threadID := "thr_1"
	turnID := "turn_1_server"

	params := map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"itemId":   "it_msg",
		"delta":    "hi",
	}
	raw, _ := json.Marshal(params)
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "item/agentMessage/delta",
		Params: raw,
	})

	got := uiPartTypes(state)
	if len(got) != 2 || got[0] != "text-start" || got[1] != "text-delta" {
		t.Fatalf("expected [text-start text-delta], got %v", got)
	}
}

func TestCodex_Mapping_ReasoningSummaryDelta_EmitsReasoningStartThenDelta(t *testing.T) {
	cc := &CodexClient{}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_1")
	attachTestTurn(state, portal)
	threadID := "thr_1"
	turnID := "turn_1_server"

	params := map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"delta":    "think",
	}
	raw, _ := json.Marshal(params)
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "item/reasoning/summaryTextDelta",
		Params: raw,
	})

	got := uiPartTypes(state)
	if len(got) != 2 || got[0] != "reasoning-start" || got[1] != "reasoning-delta" {
		t.Fatalf("expected [reasoning-start reasoning-delta], got %v", got)
	}
}

func TestCodex_Mapping_ItemStartedCommandExecution_EmitsToolInputStartAndAvailable(t *testing.T) {
	cc := &CodexClient{}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_1")
	attachTestTurn(state, portal)
	threadID := "thr_1"
	turnID := "turn_1_server"

	item := map[string]any{
		"type":    "commandExecution",
		"id":      "it_cmd",
		"command": []string{"ls", "-la"},
		"cwd":     "/tmp",
		"status":  "inProgress",
	}
	params := map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"item":     item,
	}
	raw, _ := json.Marshal(params)
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "item/started",
		Params: raw,
	})

	got := uiPartTypes(state)
	if len(got) != 2 || got[0] != "tool-input-start" || got[1] != "tool-input-available" {
		t.Fatalf("expected [tool-input-start tool-input-available], got %v", got)
	}
}

func TestCodex_Mapping_CommandOutputDelta_IsBuffered(t *testing.T) {
	cc := &CodexClient{}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_1")
	attachTestTurn(state, portal)
	threadID := "thr_1"
	turnID := "turn_1_server"

	raw1, _ := json.Marshal(map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"itemId":   "it_cmd",
		"delta":    "hello",
	})
	raw2, _ := json.Marshal(map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"itemId":   "it_cmd",
		"delta":    " world",
	})
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "item/commandExecution/outputDelta",
		Params: raw1,
	})
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "item/commandExecution/outputDelta",
		Params: raw2,
	})

	uiMessage := streamui.SnapshotCanonicalUIMessage(state.turn.UIState())
	parts := agentremote.NormalizeUIParts(uiMessage["parts"])
	var gotOutputs []string
	for _, part := range parts {
		if part["type"] != "tool-output-available" {
			continue
		}
		if out, ok := part["output"].(string); ok {
			gotOutputs = append(gotOutputs, out)
		}
	}
	if len(gotOutputs) < 2 {
		t.Fatalf("expected at least 2 tool outputs, got %v", gotOutputs)
	}
	if gotOutputs[len(gotOutputs)-1] != "hello world" {
		t.Fatalf("expected buffered output 'hello world', got %q", gotOutputs[len(gotOutputs)-1])
	}
}

func TestCodex_Mapping_TurnDiffUpdated_EmitsToolOutput(t *testing.T) {
	cc := &CodexClient{}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_1")
	attachTestTurn(state, portal)
	threadID := "thr_1"
	turnID := "turn_1_server"

	raw, _ := json.Marshal(map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"diff":     "diff --git a/x b/x",
	})
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "turn/diff/updated",
		Params: raw,
	})

	// tool-input-start, tool-input-available, tool-output-available
	got := uiPartTypes(state)
	if len(got) < 3 {
		t.Fatalf("expected >=3 parts, got %v", got)
	}
	if got[0] != "tool-input-start" || got[1] != "tool-input-available" || got[2] != "tool-output-available" {
		t.Fatalf("unexpected part types: %v", got)
	}
}

func TestCodex_Mapping_ContextCompaction_EmitsToolParts(t *testing.T) {
	cc := &CodexClient{}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_1")
	attachTestTurn(state, portal)
	threadID := "thr_1"
	turnID := "turn_1_server"

	itemStarted := map[string]any{"type": "contextCompaction", "id": "it_cc"}
	rawStarted, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": itemStarted})
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "item/started",
		Params: rawStarted,
	})

	itemCompleted := map[string]any{"type": "contextCompaction", "id": "it_cc"}
	rawCompleted, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": itemCompleted})
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{
		Method: "item/completed",
		Params: rawCompleted,
	})

	// started => tool-input-start/tool-input-available, completed => tool-output-available
	got := uiPartTypes(state)
	if len(got) < 3 {
		t.Fatalf("expected >=3 parts, got %v", got)
	}
	if got[0] != "tool-input-start" || got[1] != "tool-input-available" || got[2] != "tool-output-available" {
		t.Fatalf("unexpected part types: %v", got)
	}
}

func TestCodex_Mapping_ReviewMode_EmitsReviewToolOutput(t *testing.T) {
	cc := &CodexClient{}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := newHookableStreamingState("turn_1")
	attachTestTurn(state, portal)
	threadID := "thr_1"
	turnID := "turn_1_server"

	rawStarted, _ := json.Marshal(map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"item":     map[string]any{"type": "enteredReviewMode", "id": "it_review", "review": "current changes"},
	})
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{Method: "item/started", Params: rawStarted})

	rawCompleted, _ := json.Marshal(map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"item":     map[string]any{"type": "exitedReviewMode", "id": "it_review", "review": "Looks good"},
	})
	cc.handleNotif(context.Background(), portal, nil, state, "model", threadID, turnID, codexNotif{Method: "item/completed", Params: rawCompleted})

	gotTypes := uiPartTypes(state)
	// At least one tool output should be present.
	seenOutput := false
	for _, typ := range gotTypes {
		if typ == "tool-output-available" {
			seenOutput = true
			break
		}
	}
	if !seenOutput {
		t.Fatalf("expected tool-output-available, got %v", gotTypes)
	}
}
