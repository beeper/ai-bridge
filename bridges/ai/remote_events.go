package ai

import (
	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// -----------------------------------------------------------------------
// AIRemoteMessageRemove — for redacting messages
// -----------------------------------------------------------------------

var _ bridgev2.RemoteMessageRemove = (*AIRemoteMessageRemove)(nil)

// AIRemoteMessageRemove is a RemoteMessageRemove for redacting AI or user messages.
type AIRemoteMessageRemove struct {
	portal        networkid.PortalKey
	sender        bridgev2.EventSender
	targetMessage networkid.MessageID
}

func (r *AIRemoteMessageRemove) GetType() bridgev2.RemoteEventType {
	return bridgev2.RemoteEventMessageRemove
}

func (r *AIRemoteMessageRemove) GetPortalKey() networkid.PortalKey {
	return r.portal
}

func (r *AIRemoteMessageRemove) AddLogContext(c zerolog.Context) zerolog.Context {
	return c.Str("ai_remove_target", string(r.targetMessage))
}

func (r *AIRemoteMessageRemove) GetSender() bridgev2.EventSender {
	return r.sender
}

func (r *AIRemoteMessageRemove) GetTargetMessage() networkid.MessageID {
	return r.targetMessage
}
