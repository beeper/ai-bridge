package codex

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
)

// loggerFromContext returns the logger from the context if available,
// otherwise falls back to the provided logger.
func loggerFromContext(ctx context.Context, fallback *zerolog.Logger) *zerolog.Logger {
	if ctx != nil {
		if ctxLog := zerolog.Ctx(ctx); ctxLog != nil && ctxLog.GetLevel() != zerolog.Disabled {
			return ctxLog
		}
	}
	return fallback
}

func unsupportedMessageStatus(err error) error {
	return bridgev2.WrapErrorInStatus(err).
		WithStatus(event.MessageStatusFail).
		WithErrorReason(event.MessageStatusUnsupported).
		WithIsCertain(true).
		WithSendNotice(true).
		WithErrorAsMessage()
}

func messageSendStatusError(err error, message string, reason event.MessageStatusReason) error {
	if err == nil {
		if message == "" {
			err = errors.New("message send failed")
		} else {
			err = errors.New(message)
		}
	}
	st := bridgev2.WrapErrorInStatus(err).WithSendNotice(true)
	st = st.WithStatus(messageStatusForError(err))
	if reason != "" {
		st = st.WithErrorReason(reason)
	} else {
		st = st.WithErrorReason(messageStatusReasonForError(err))
	}
	if message != "" {
		st = st.WithMessage(message)
	} else {
		st = st.WithErrorAsMessage()
	}
	return st
}

// brokenLoginClient keeps invalid logins loadable/deletable.
type brokenLoginClient struct {
	UserLogin *bridgev2.UserLogin
	Reason    string
}

var _ bridgev2.NetworkAPI = (*brokenLoginClient)(nil)

func (c *brokenLoginClient) Connect(ctx context.Context) {
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

func (c *brokenLoginClient) Disconnect()                                {}
func (c *brokenLoginClient) IsLoggedIn() bool                           { return false }
func (c *brokenLoginClient) LogoutRemote(ctx context.Context)           {}
func (c *brokenLoginClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return false
}
func (c *brokenLoginClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	return nil, bridgev2.ErrNotLoggedIn
}
func (c *brokenLoginClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	return nil, bridgev2.ErrNotLoggedIn
}
func (c *brokenLoginClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	return &event.RoomFeatures{}
}
func (c *brokenLoginClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	return nil, bridgev2.ErrNotLoggedIn
}

func purgeLoginDataBestEffort(ctx context.Context, login *bridgev2.UserLogin) {
	_ = ctx
	_ = login
}

