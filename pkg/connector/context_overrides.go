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
	if model, ok := ctx.Value(contextKeyModelOverride{}).(string); ok && model != "" {
		return model, true
	}
	return "", false
}
