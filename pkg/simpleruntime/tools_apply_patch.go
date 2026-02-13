package connector

import (
	"context"
	"errors"
)

func executeApplyPatch(ctx context.Context, args map[string]any) (string, error) {
	_ = ctx
	_ = args
	return "", errors.New("apply_patch is not available in the simple bridge")
}
