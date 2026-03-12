package agentremote

import (
	"context"
	"sync"

	"maunium.net/go/mautrix/bridgev2"
)

type ClientBase struct {
	BaseReactionHandler
	BaseStreamState

	loginMu sync.RWMutex
	login   *bridgev2.UserLogin
}

func (c *ClientBase) InitClientBase(login *bridgev2.UserLogin, target ReactionTarget) {
	c.SetUserLogin(login)
	c.BaseReactionHandler.Target = target
	c.InitStreamState()
}

func (c *ClientBase) SetUserLogin(login *bridgev2.UserLogin) {
	c.loginMu.Lock()
	c.login = login
	c.loginMu.Unlock()
}

func (c *ClientBase) GetUserLogin() *bridgev2.UserLogin {
	if c == nil {
		return nil
	}
	c.loginMu.RLock()
	defer c.loginMu.RUnlock()
	return c.login
}

func (c *ClientBase) Login() *bridgev2.UserLogin {
	return c.GetUserLogin()
}

func (c *ClientBase) BackgroundContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	login := c.GetUserLogin()
	if login != nil && login.Bridge != nil && login.Bridge.BackgroundCtx != nil {
		return login.Bridge.BackgroundCtx
	}
	return context.Background()
}
