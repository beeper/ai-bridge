package codex

import (
	"context"
	"fmt"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote"
)

// sendViaPortal sends a pre-built message through bridgev2's QueueRemoteEvent pipeline.
func (cc *CodexClient) sendViaPortal(
	_ context.Context,
	portal *bridgev2.Portal,
	converted *bridgev2.ConvertedMessage,
	msgID networkid.MessageID,
) (id.EventID, networkid.MessageID, error) {
	return cc.sendViaPortalWithOrdering(portal, converted, msgID, time.Time{}, 0)
}

func (cc *CodexClient) sendViaPortalWithOrdering(
	portal *bridgev2.Portal,
	converted *bridgev2.ConvertedMessage,
	msgID networkid.MessageID,
	timestamp time.Time,
	streamOrder int64,
) (id.EventID, networkid.MessageID, error) {
	return agentremote.SendViaPortal(agentremote.SendViaPortalParams{
		Login:       cc.UserLogin,
		Portal:      portal,
		Sender:      cc.senderForPortal(),
		IDPrefix:    "codex",
		LogKey:      "codex_msg_id",
		MsgID:       msgID,
		Timestamp:   timestamp,
		StreamOrder: streamOrder,
		Converted:   converted,
	})
}

// getCodexIntentForPortal resolves the Matrix intent for the Codex ghost.
// Use this when you need an intent for non-message operations (e.g. UploadMedia, debounced edits).
func (cc *CodexClient) getCodexIntentForPortal(
	ctx context.Context,
	portal *bridgev2.Portal,
	evtType bridgev2.RemoteEventType,
) (bridgev2.MatrixAPI, error) {
	sender := cc.senderForPortal()
	intent, ok := portal.GetIntentFor(ctx, sender, cc.UserLogin, evtType)
	if !ok {
		return nil, fmt.Errorf("intent resolution failed")
	}
	return intent, nil
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
		sender.Sender = humanUserID(cc.UserLogin.ID)
		sender.SenderLogin = cc.UserLogin.ID
	}
	return sender
}
