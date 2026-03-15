package codex

import (
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

// sendViaPortal sends a pre-built message through bridgev2's QueueRemoteEvent pipeline.
func (cc *CodexClient) sendViaPortal(
	portal *bridgev2.Portal,
	converted *bridgev2.ConvertedMessage,
	msgID networkid.MessageID,
	timestamp time.Time,
	streamOrder int64,
) (id.EventID, networkid.MessageID, error) {
	return cc.ClientBase.SendViaPortalWithOptions(portal, cc.senderForPortal(), msgID, timestamp, streamOrder, converted)
}

// senderForPortal returns the EventSender for the Codex ghost.
func (cc *CodexClient) senderForPortal() bridgev2.EventSender {
	sender := bridgev2.EventSender{Sender: codexGhostID}
	if cc != nil && cc.UserLogin != nil {
		sender.SenderLogin = cc.UserLogin.ID
	}
	return sender
}

func (cc *CodexClient) senderForHuman() bridgev2.EventSender {
	sender := bridgev2.EventSender{IsFromMe: true}
	if cc != nil && cc.UserLogin != nil {
		sender.Sender = cc.HumanUserID()
		sender.SenderLogin = cc.UserLogin.ID
	}
	return sender
}
