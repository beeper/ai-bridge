package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/core/aityping"
)

type TypingContext = aityping.TypingContext

var WithTypingContext = aityping.WithTypingContext

func typingContextFromContext(ctx context.Context) *TypingContext {
	return aityping.TypingContextFromContext(ctx)
}
