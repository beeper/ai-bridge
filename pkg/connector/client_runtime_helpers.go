package connector

import (
	"context"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
)

func (oc *AIClient) Log() *zerolog.Logger {
	if oc == nil {
		logger := zerolog.Nop()
		return &logger
	}
	return &oc.log
}

func (oc *AIClient) Login() *bridgev2.UserLogin {
	if oc == nil {
		return nil
	}
	return oc.UserLogin
}

func (oc *AIClient) BackgroundContext(ctx context.Context) context.Context {
	if oc == nil {
		return ctx
	}
	return oc.backgroundContext(ctx)
}
