package bridgeadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type testApprovalFlowData struct {
}

func TestApprovalFlow_FinishResolvedQueuesEditAndPlaceholderCleanup(t *testing.T) {
	owner := id.UserID("@owner:example.com")
	roomID := id.RoomID("!room:example.com")
	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: roomID}}
	login := &bridgev2.UserLogin{
		UserLogin: &database.UserLogin{
			ID:       networkid.UserLoginID("login"),
			UserMXID: owner,
		},
		Bridge: &bridgev2.Bridge{},
	}

	flow := NewApprovalFlow(ApprovalFlowConfig[*testApprovalFlowData]{
		Login: func() *bridgev2.UserLogin { return login },
	})
	flow.testResolvePortal = func(ctx context.Context, login *bridgev2.UserLogin, roomID id.RoomID) (*bridgev2.Portal, error) {
		_ = ctx
		_ = login
		_ = roomID
		return portal, nil
	}

	editCh := make(chan ApprovalDecisionPayload, 1)
	cleanupCh := make(chan struct{}, 1)
	flow.testEditPromptToResolvedState = func(ctx context.Context, login *bridgev2.UserLogin, portal *bridgev2.Portal, sender bridgev2.EventSender, prompt ApprovalPromptRegistration, decision ApprovalDecisionPayload) {
		_ = ctx
		_ = login
		_ = portal
		_ = sender
		if prompt.PromptMessageID == "" {
			t.Errorf("expected prompt message id to be set")
		}
		editCh <- decision
	}
	flow.testRedactPromptPlaceholderReacts = func(ctx context.Context, login *bridgev2.UserLogin, portal *bridgev2.Portal, sender bridgev2.EventSender, prompt ApprovalPromptRegistration) error {
		_ = ctx
		_ = login
		_ = portal
		_ = sender
		_ = prompt
		cleanupCh <- struct{}{}
		return nil
	}

	if _, created := flow.Register("approval-1", time.Minute, &testApprovalFlowData{}); !created {
		t.Fatalf("expected pending approval to be created")
	}
	flow.mu.Lock()
	flow.registerPromptLocked(ApprovalPromptRegistration{
		ApprovalID:      "approval-1",
		RoomID:          roomID,
		OwnerMXID:       owner,
		ToolCallID:      "tool-1",
		ToolName:        "exec",
		PromptEventID:   id.EventID("$prompt"),
		PromptMessageID: networkid.MessageID("msg-1"),
		PromptSenderID:  networkid.UserID("ghost:approval"),
		Options:         DefaultApprovalOptions(),
	})
	flow.mu.Unlock()

	flow.FinishResolved("approval-1", ApprovalDecisionPayload{
		ApprovalID: "approval-1",
		Approved:   true,
		Reason:     "allow_once",
	})
	if pending := flow.Get("approval-1"); pending != nil {
		t.Fatalf("expected pending approval to be finalized")
	}
	flow.mu.Lock()
	_, stillPrompt := flow.promptsByApproval["approval-1"]
	flow.mu.Unlock()
	if stillPrompt {
		t.Fatalf("expected prompt registration to be finalized")
	}

	select {
	case <-cleanupCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for placeholder cleanup scheduling")
	}

	select {
	case decision := <-editCh:
		if !decision.Approved {
			t.Fatalf("expected approved decision, got %#v", decision)
		}
		if decision.Reason != "allow_once" {
			t.Fatalf("expected reason allow_once, got %#v", decision)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for prompt edit scheduling")
	}
}

func TestIsApprovalPlaceholderReaction_ExcludesUserReaction(t *testing.T) {
	prompt := ApprovalPromptRegistration{
		PromptSenderID: networkid.UserID("ghost:approval"),
	}
	sender := bridgev2.EventSender{Sender: networkid.UserID("ghost:approval")}

	if !isApprovalPlaceholderReaction(&database.Reaction{SenderID: networkid.UserID("ghost:approval")}, prompt, sender) {
		t.Fatalf("expected bridge-authored reaction to be placeholder")
	}
	if isApprovalPlaceholderReaction(&database.Reaction{SenderID: MatrixSenderID(id.UserID("@owner:example.com"))}, prompt, sender) {
		t.Fatalf("did not expect user reaction to be placeholder")
	}
}

func TestApprovalFlow_HandleReaction_DeliveryErrorKeepsPending(t *testing.T) {
	owner := id.UserID("@owner:example.com")
	roomID := id.RoomID("!room:example.com")
	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: roomID}}
	login := &bridgev2.UserLogin{
		UserLogin: &database.UserLogin{
			ID:       networkid.UserLoginID("login"),
			UserMXID: owner,
		},
		Bridge: &bridgev2.Bridge{},
	}

	var redacted bool
	flow := NewApprovalFlow(ApprovalFlowConfig[*testApprovalFlowData]{
		Login: func() *bridgev2.UserLogin { return login },
		DeliverDecision: func(ctx context.Context, portal *bridgev2.Portal, pending *Pending[*testApprovalFlowData], decision ApprovalDecisionPayload) error {
			_ = ctx
			_ = portal
			_ = pending
			_ = decision
			return errors.New("boom")
		},
	})
	flow.testRedactSingleReaction = func(msg *bridgev2.MatrixReaction) {
		_ = msg
		redacted = true
	}
	if _, created := flow.Register("approval-1", time.Minute, &testApprovalFlowData{}); !created {
		t.Fatalf("expected pending approval to be created")
	}
	flow.mu.Lock()
	flow.registerPromptLocked(ApprovalPromptRegistration{
		ApprovalID:    "approval-1",
		RoomID:        roomID,
		OwnerMXID:     owner,
		ToolCallID:    "tool-1",
		PromptEventID: id.EventID("$prompt"),
		Options:       DefaultApprovalOptions(),
	})
	flow.mu.Unlock()

	msg := &bridgev2.MatrixReaction{
		MatrixEventBase: bridgev2.MatrixEventBase[*event.ReactionEventContent]{
			Event:  &event.Event{ID: id.EventID("$reaction"), Sender: owner},
			Portal: portal,
		},
	}
	if !flow.HandleReaction(context.Background(), msg, id.EventID("$prompt"), ApprovalReactionKeyAllowOnce) {
		t.Fatalf("expected approval reaction to be handled")
	}
	if flow.Get("approval-1") == nil {
		t.Fatalf("expected pending approval to remain after delivery error")
	}
	if !redacted {
		t.Fatalf("expected failed user reaction to be redacted")
	}
}

func TestApprovalFlow_ResolveExternalMirrorsRemoteDecision(t *testing.T) {
	owner := id.UserID("@owner:example.com")
	roomID := id.RoomID("!room:example.com")
	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: roomID}}
	login := &bridgev2.UserLogin{
		UserLogin: &database.UserLogin{
			ID:       networkid.UserLoginID("login"),
			UserMXID: owner,
		},
		Bridge: &bridgev2.Bridge{},
	}

	flow := NewApprovalFlow(ApprovalFlowConfig[*testApprovalFlowData]{
		Login: func() *bridgev2.UserLogin { return login },
	})
	flow.testResolvePortal = func(ctx context.Context, login *bridgev2.UserLogin, roomID id.RoomID) (*bridgev2.Portal, error) {
		_ = ctx
		_ = login
		_ = roomID
		return portal, nil
	}

	mirrorCh := make(chan string, 1)
	flow.testMirrorRemoteDecisionReaction = func(ctx context.Context, login *bridgev2.UserLogin, portal *bridgev2.Portal, sender bridgev2.EventSender, prompt ApprovalPromptRegistration, reactionKey string) {
		_ = ctx
		_ = login
		_ = portal
		if sender.Sender != MatrixSenderID(owner) {
			t.Errorf("expected mirrored reaction sender to be owner, got %q", sender.Sender)
		}
		if prompt.PromptMessageID == "" {
			t.Errorf("expected prompt message id to be set")
		}
		mirrorCh <- reactionKey
	}
	flow.testEditPromptToResolvedState = func(ctx context.Context, login *bridgev2.UserLogin, portal *bridgev2.Portal, sender bridgev2.EventSender, prompt ApprovalPromptRegistration, decision ApprovalDecisionPayload) {
	}
	flow.testRedactPromptPlaceholderReacts = func(ctx context.Context, login *bridgev2.UserLogin, portal *bridgev2.Portal, sender bridgev2.EventSender, prompt ApprovalPromptRegistration) error {
		return nil
	}

	if _, created := flow.Register("approval-1", time.Minute, &testApprovalFlowData{}); !created {
		t.Fatalf("expected pending approval to be created")
	}
	flow.mu.Lock()
	flow.registerPromptLocked(ApprovalPromptRegistration{
		ApprovalID:      "approval-1",
		RoomID:          roomID,
		OwnerMXID:       owner,
		ToolCallID:      "tool-1",
		PromptEventID:   id.EventID("$prompt"),
		PromptMessageID: networkid.MessageID("msg-1"),
		Options:         DefaultApprovalOptions(),
	})
	flow.mu.Unlock()

	flow.ResolveExternal(context.Background(), "approval-1", ApprovalDecisionPayload{
		ApprovalID: "approval-1",
		Approved:   true,
		Always:     true,
		Reason:     "allow-always",
	})

	select {
	case key := <-mirrorCh:
		if key != ApprovalReactionKeyAllowAlways {
			t.Fatalf("expected allow_always reaction key, got %q", key)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for mirrored remote reaction")
	}
}
