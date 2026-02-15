package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/core/aityping"
)

type TypingContext struct {
	IsGroup      bool
	WasMentioned bool
}

type typingContextKey struct{}

func WithTypingContext(ctx context.Context, typing *TypingContext) context.Context {
	if ctx == nil || typing == nil {
		return ctx
	}
	return context.WithValue(ctx, typingContextKey{}, typing)
}

func typingContextFromContext(ctx context.Context) *TypingContext {
	if ctx == nil {
		return nil
	}
	if local, ok := ctx.Value(typingContextKey{}).(*TypingContext); ok && local != nil {
		return local
	}
	shared := aityping.TypingContextFromContext(ctx)
	if shared == nil {
		return nil
	}
	return &TypingContext{
		IsGroup:      shared.IsGroup,
		WasMentioned: shared.WasMentioned,
	}
}
