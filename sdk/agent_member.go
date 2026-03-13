package sdk

import (
	"context"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// AgentMember represents an AI agent ghost in the bridge.
type AgentMember struct {
	ID          string
	Name        string
	AvatarURL   string
	IsBot       bool
	Identifiers []string
}

// EnsureGhost ensures the ghost user exists in the bridge database.
func (a *AgentMember) EnsureGhost(ctx context.Context, login *bridgev2.UserLogin) error {
	if a == nil || login == nil || login.Bridge == nil {
		return nil
	}
	ghost, err := login.Bridge.GetGhostByID(ctx, networkid.UserID(a.ID))
	if err != nil {
		return err
	}
	if ghost == nil {
		return nil
	}
	info := a.UserInfo()
	ghost.UpdateInfo(ctx, info)
	return nil
}

// EventSender returns the bridgev2.EventSender for this agent member.
func (a *AgentMember) EventSender(loginID networkid.UserLoginID) bridgev2.EventSender {
	if a == nil {
		return bridgev2.EventSender{}
	}
	return bridgev2.EventSender{
		Sender:      networkid.UserID(a.ID),
		SenderLogin: loginID,
	}
}

// UserInfo returns a bridgev2.UserInfo for this agent member.
func (a *AgentMember) UserInfo() *bridgev2.UserInfo {
	if a == nil {
		return nil
	}
	return &bridgev2.UserInfo{
		Name:        ptr.Ptr(a.Name),
		IsBot:       ptr.Ptr(a.IsBot),
		Identifiers: a.Identifiers,
	}
}
