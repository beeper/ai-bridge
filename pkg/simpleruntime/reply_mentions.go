package connector

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"
)

func (oc *AIClient) isReplyToBot(ctx context.Context, portal *bridgev2.Portal, replyTo id.EventID) bool {
	if oc == nil || portal == nil || replyTo == "" || oc.UserLogin == nil || oc.UserLogin.Bridge == nil {
		return false
	}
	msg, err := oc.UserLogin.Bridge.DB.Message.GetPartByMXID(ctx, replyTo)
	if err != nil || msg == nil {
		return false
	}
	sender := strings.TrimSpace(string(msg.SenderID))
	if sender == "" {
		return false
	}
	return strings.HasPrefix(sender, "model-") || strings.HasPrefix(sender, "agent-")
}
