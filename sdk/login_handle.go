package sdk

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// LoginHandle wraps a UserLogin and provides convenience methods for creating
// conversations and accessing login state.
type LoginHandle struct {
	login  *bridgev2.UserLogin
	client *sdkClient
}

func newLoginHandle(login *bridgev2.UserLogin, client *sdkClient) *LoginHandle {
	return &LoginHandle{
		login:  login,
		client: client,
	}
}

// Conversation returns a Conversation for the given portal ID.
func (l *LoginHandle) Conversation(ctx context.Context, portalID string) *Conversation {
	if l.login == nil || l.login.Bridge == nil {
		return newConversation(ctx, nil, l.login, bridgev2.EventSender{}, l.client)
	}
	portalKey := networkid.PortalKey{
		ID: networkid.PortalID(portalID),
	}
	if l.login != nil {
		portalKey.Receiver = l.login.ID
	}
	portal, err := l.login.Bridge.GetExistingPortalByKey(ctx, portalKey)
	if err != nil || portal == nil {
		return newConversation(ctx, nil, l.login, bridgev2.EventSender{}, l.client)
	}
	return newConversation(ctx, portal, l.login, bridgev2.EventSender{}, l.client)
}

// ConversationByPortal returns a Conversation for the given bridgev2.Portal.
func (l *LoginHandle) ConversationByPortal(ctx context.Context, portal *bridgev2.Portal) *Conversation {
	return newConversation(ctx, portal, l.login, bridgev2.EventSender{}, l.client)
}

// UserLogin returns the underlying bridgev2.UserLogin.
func (l *LoginHandle) UserLogin() *bridgev2.UserLogin {
	return l.login
}
