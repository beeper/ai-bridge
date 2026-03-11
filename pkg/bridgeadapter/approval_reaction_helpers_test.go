package bridgeadapter

import (
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func TestIsApprovalPlaceholderReaction_UsesPromptSenderID(t *testing.T) {
	prompt := ApprovalPromptRegistration{
		PromptSenderID: networkid.UserID("mxid:@ghost:example.com"),
	}
	reaction := &database.Reaction{
		SenderID: networkid.UserID("mxid:@ghost:example.com"),
	}
	if !isApprovalPlaceholderReaction(reaction, prompt, bridgev2.EventSender{}) {
		t.Fatalf("expected reaction to be treated as placeholder")
	}
	reaction.SenderID = networkid.UserID("mxid:@owner:example.com")
	if isApprovalPlaceholderReaction(reaction, prompt, bridgev2.EventSender{}) {
		t.Fatalf("expected non-prompt sender reaction to be preserved")
	}
}

func TestIsApprovalPlaceholderReaction_FallsBackToSender(t *testing.T) {
	prompt := ApprovalPromptRegistration{}
	reaction := &database.Reaction{
		SenderID: networkid.UserID("mxid:@ghost:example.com"),
	}
	if !isApprovalPlaceholderReaction(reaction, prompt, bridgev2.EventSender{Sender: networkid.UserID("mxid:@ghost:example.com")}) {
		t.Fatalf("expected fallback sender to match placeholder reaction")
	}
}
