package brokenlogin

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
)

// Client keeps an unusable login loadable/deletable by implementing NetworkAPI.
type Client struct {
	UserLogin *bridgev2.UserLogin
	Reason    string
}

var _ bridgev2.NetworkAPI = (*Client)(nil)

func New(userLogin *bridgev2.UserLogin, reason string) bridgev2.NetworkAPI {
	return &Client{
		UserLogin: userLogin,
		Reason:    reason,
	}
}

func (c *Client) Connect(context.Context) {
	if c == nil || c.UserLogin == nil || c.UserLogin.BridgeState == nil {
		return
	}
	msg := c.Reason
	if msg == "" {
		msg = "Login is not usable. Sign in again or remove this account."
	}
	c.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateBadCredentials,
		Message:    msg,
	})
}

func (c *Client) Disconnect() {}

func (c *Client) IsLoggedIn() bool { return false }

func (c *Client) LogoutRemote(context.Context) {}

func (c *Client) IsThisUser(context.Context, networkid.UserID) bool { return false }

func (c *Client) GetChatInfo(context.Context, *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	return nil, bridgev2.ErrNotLoggedIn
}

func (c *Client) GetUserInfo(context.Context, *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	return nil, bridgev2.ErrNotLoggedIn
}

func (c *Client) GetCapabilities(context.Context, *bridgev2.Portal) *event.RoomFeatures {
	return &event.RoomFeatures{}
}

func (c *Client) HandleMatrixMessage(context.Context, *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	return nil, bridgev2.ErrNotLoggedIn
}
