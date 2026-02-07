package connector

import (
	"context"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/id"
)

func TestCodex_Mapping_AgentMessageDelta_EmitsTextStartThenDelta(t *testing.T) {
	cc := &CodexClient{}
	var got []string
	cc.streamEventHook = func(turnID string, seq int, content map[string]any, txnID string) {
		_ = turnID
		_ = seq
		_ = txnID
		part, _ := content["part"].(map[string]any)
		typ, _ := part["type"].(string)
		got = append(got, typ)
	}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := &streamingState{turnID: "turn_1"}
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

	if len(got) != 2 || got[0] != "text-start" || got[1] != "text-delta" {
		t.Fatalf("expected [text-start text-delta], got %v", got)
	}
}

func TestCodex_Mapping_ReasoningSummaryDelta_EmitsReasoningStartThenDelta(t *testing.T) {
	cc := &CodexClient{}
	var got []string
	cc.streamEventHook = func(turnID string, seq int, content map[string]any, txnID string) {
		_ = turnID
		_ = seq
		_ = txnID
		part, _ := content["part"].(map[string]any)
		typ, _ := part["type"].(string)
		got = append(got, typ)
	}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := &streamingState{turnID: "turn_1"}
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

	if len(got) != 2 || got[0] != "reasoning-start" || got[1] != "reasoning-delta" {
		t.Fatalf("expected [reasoning-start reasoning-delta], got %v", got)
	}
}

func TestCodex_Mapping_ItemStartedCommandExecution_EmitsToolInputStartAndAvailable(t *testing.T) {
	cc := &CodexClient{}
	var got []string
	cc.streamEventHook = func(turnID string, seq int, content map[string]any, txnID string) {
		_ = turnID
		_ = seq
		_ = txnID
		part, _ := content["part"].(map[string]any)
		typ, _ := part["type"].(string)
		got = append(got, typ)
	}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	state := &streamingState{turnID: "turn_1"}
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

	if len(got) != 2 || got[0] != "tool-input-start" || got[1] != "tool-input-available" {
		t.Fatalf("expected [tool-input-start tool-input-available], got %v", got)
	}
}
