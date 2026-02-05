package connector

import (
	"context"
	"strings"
)

type contextKeyModelOverride struct{}

func withModelOverride(ctx context.Context, model string) context.Context {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKeyModelOverride{}, trimmed)
}

func modelOverrideFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if value := ctx.Value(contextKeyModelOverride{}); value != nil {
		if model, ok := value.(string); ok && strings.TrimSpace(model) != "" {
			return strings.TrimSpace(model), true
		}
	}
	return "", false
}
