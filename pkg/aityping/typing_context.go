package aityping

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/aiutil"
)

// TypingContext carries group-chat and mention state for typing decisions.
type TypingContext struct {
	IsGroup      bool
	WasMentioned bool
}

type typingContextKey struct{}

// WithTypingContext attaches a TypingContext to a context.
func WithTypingContext(ctx context.Context, typing *TypingContext) context.Context {
	if ctx == nil || typing == nil {
		return ctx
	}
	return context.WithValue(ctx, typingContextKey{}, typing)
}

// TypingContextFromContext retrieves the TypingContext from a context.
func TypingContextFromContext(ctx context.Context) *TypingContext {
	return aiutil.ContextValue[*TypingContext](ctx, typingContextKey{})
}
