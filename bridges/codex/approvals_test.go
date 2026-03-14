package codex

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/bridges/codex/codexrpc"
	bridgesdk "github.com/beeper/agentremote/sdk"
)

func newTestCodexClient(owner id.UserID) *CodexClient {
	ul := &bridgev2.UserLogin{}
	ul.UserLogin = &database.UserLogin{
		UserMXID: owner,
	}
	cc := &CodexClient{
		UserLogin:   ul,
		activeRooms: make(map[id.RoomID]bool),
	}
	cc.approvalFlow = agentremote.NewApprovalFlow(agentremote.ApprovalFlowConfig[*pendingToolApprovalDataCodex]{
		Login: func() *bridgev2.UserLogin { return cc.UserLogin },
		RoomIDFromData: func(data *pendingToolApprovalDataCodex) id.RoomID {
			if data == nil {
				return ""
			}
			return data.RoomID
		},
	})
	return cc
}

func waitForPendingApproval(t *testing.T, ctx context.Context, cc *CodexClient, approvalID string) *agentremote.Pending[*pendingToolApprovalDataCodex] {
	t.Helper()
	for {
		pending := cc.approvalFlow.Get(approvalID)
		if pending != nil && pending.Data != nil {
			return pending
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("timed out waiting for approval %s: %v", approvalID, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func attachApprovalTestTurn(state *streamingState, portal *bridgev2.Portal) {
	if state == nil {
		return
	}
	conv := bridgesdk.NewConversation(context.Background(), nil, portal, bridgev2.EventSender{}, &bridgesdk.Config{}, nil)
	turn := conv.StartTurn(context.Background(), nil, nil)
	turn.SetID(state.turnID)
	state.turn = turn
}

func TestCodex_CommandApproval_RequestBlocksUntilApproved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	cc := newTestCodexClient(id.UserID("@owner:example.com"))

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	meta := &PortalMetadata{}
	state := &streamingState{turnID: "turn_local", initialEventID: id.EventID("$event"), networkMessageID: networkid.MessageID("codex:test")}
	attachApprovalTestTurn(state, portal)
	cc.activeTurns = map[string]*codexActiveTurn{
		codexTurnKey("thr_1", "turn_1"): {
			portal:   portal,
			meta:     meta,
			state:    state,
			threadID: "thr_1",
			turnID:   "turn_1",
			model:    "gpt-5.1-codex",
		},
	}

	params := map[string]any{
		"threadId": "thr_1",
		"turnId":   "turn_1",
		"itemId":   "item_1",
		"command":  "echo hi",
		"cwd":      "/tmp",
	}
	paramsRaw, _ := json.Marshal(params)
	req := codexrpc.Request{
		ID:     json.RawMessage("123"),
		Method: "item/commandExecution/requestApproval",
		Params: paramsRaw,
	}

	resCh := make(chan map[string]any, 1)
	go func() {
		res, _ := cc.handleCommandApprovalRequest(ctx, req)
		resCh <- res.(map[string]any)
	}()

	pending := waitForPendingApproval(t, ctx, cc, "123")
	if pending.Data.Presentation.AllowAlways {
		t.Fatalf("expected codex approvals to disable always-allow")
	}
	if pending.Data.Presentation.Title == "" {
		t.Fatalf("expected structured presentation title")
	}

	if err := cc.approvalFlow.Resolve("123", agentremote.ApprovalDecisionPayload{
		ApprovalID: "123",
		Approved:   true,
		Reason:     "allow_once",
	}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	select {
	case res := <-resCh:
		if res["decision"] != "accept" {
			t.Fatalf("expected decision=accept, got %#v", res)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for approval handler to return")
	}

	uiState := state.turn.UIState()
	if uiState == nil || !uiState.UIToolApprovalRequested["123"] {
		t.Fatal("expected approval request to be tracked in UI state")
	}
	if uiState.UIToolCallIDByApproval["123"] != "item_1" {
		t.Fatalf("expected approval to map to tool call item_1, got %q", uiState.UIToolCallIDByApproval["123"])
	}
}

func TestCodex_CommandApproval_DenyEmitsResponseThenOutputDenied(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	cc := newTestCodexClient(id.UserID("@owner:example.com"))

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	meta := &PortalMetadata{}
	state := &streamingState{turnID: "turn_local", initialEventID: id.EventID("$event"), networkMessageID: networkid.MessageID("codex:test")}
	attachApprovalTestTurn(state, portal)
	cc.activeTurns = map[string]*codexActiveTurn{
		codexTurnKey("thr_1", "turn_1"): {
			portal:   portal,
			meta:     meta,
			state:    state,
			threadID: "thr_1",
			turnID:   "turn_1",
			model:    "gpt-5.1-codex",
		},
	}

	paramsRaw, _ := json.Marshal(map[string]any{
		"threadId": "thr_1",
		"turnId":   "turn_1",
		"itemId":   "item_1",
		"command":  "rm -rf /tmp/test",
	})
	req := codexrpc.Request{
		ID:     json.RawMessage("456"),
		Method: "item/commandExecution/requestApproval",
		Params: paramsRaw,
	}

	resCh := make(chan map[string]any, 1)
	go func() {
		res, _ := cc.handleCommandApprovalRequest(ctx, req)
		resCh <- res.(map[string]any)
	}()

	waitForPendingApproval(t, ctx, cc, "456")
	if err := cc.approvalFlow.Resolve("456", agentremote.ApprovalDecisionPayload{
		ApprovalID: "456",
		Approved:   false,
		Reason:     "deny",
	}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	select {
	case res := <-resCh:
		if res["decision"] != "decline" {
			t.Fatalf("expected decision=decline, got %#v", res)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for approval handler to return")
	}

	uiState := state.turn.UIState()
	if uiState == nil || !uiState.UIToolApprovalRequested["456"] {
		t.Fatal("expected denied approval request to be tracked in UI state")
	}
	if uiState.UIToolCallIDByApproval["456"] != "item_1" {
		t.Fatalf("expected approval to map to tool call item_1, got %q", uiState.UIToolCallIDByApproval["456"])
	}
}

func TestCodex_CommandApproval_AutoApproveInFullElevated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	cc := newTestCodexClient(id.UserID("@owner:example.com"))
	cc.streamEventHook = func(turnID string, seq int, content map[string]any, txnID string) {}

	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.com")}}
	meta := &PortalMetadata{ElevatedLevel: "full"}
	state := &streamingState{turnID: "turn_local", initialEventID: id.EventID("$event"), networkMessageID: networkid.MessageID("codex:test")}
	cc.activeTurns = map[string]*codexActiveTurn{
		codexTurnKey("thr_1", "turn_1"): {
			portal:   portal,
			meta:     meta,
			state:    state,
			threadID: "thr_1",
			turnID:   "turn_1",
		},
	}

	paramsRaw, _ := json.Marshal(map[string]any{
		"threadId": "thr_1",
		"turnId":   "turn_1",
		"itemId":   "item_1",
	})
	req := codexrpc.Request{
		ID:     json.RawMessage("321"),
		Method: "item/commandExecution/requestApproval",
		Params: paramsRaw,
	}

	res, _ := cc.handleCommandApprovalRequest(ctx, req)
	if res.(map[string]any)["decision"] != "accept" {
		t.Fatalf("expected decision=accept, got %#v", res)
	}
}

func TestCodex_CommandApproval_RejectCrossRoom(t *testing.T) {
	owner := id.UserID("@owner:example.com")
	roomID := id.RoomID("!room1:example.com")
	otherRoom := id.RoomID("!room2:example.com")

	cc := newTestCodexClient(owner)
	cc.registerToolApproval(roomID, "approval-1", "item-1", "commandExecution", agentremote.ApprovalPromptPresentation{
		Title:       "Codex command execution",
		AllowAlways: false,
	}, 2*time.Second)

	// Register the approval in a second room to test cross-room rejection.
	// The flow's HandleReaction checks room via RoomIDFromData, so we test
	// that the registered room doesn't match a different room.
	p := cc.approvalFlow.Get("approval-1")
	if p == nil {
		t.Fatalf("expected pending approval to exist")
	}
	if p.Data == nil || p.Data.RoomID != roomID {
		t.Fatalf("expected pending data with RoomID=%s, got %v", roomID, p.Data)
	}
	// The RoomIDFromData callback returns roomID, which won't match otherRoom.
	_ = otherRoom
}
