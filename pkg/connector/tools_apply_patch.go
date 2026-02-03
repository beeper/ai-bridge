package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/beeper/ai-bridge/pkg/textfs"
)

func executeApplyPatch(ctx context.Context, args map[string]any) (string, error) {
	store, err := textFSStore(ctx)
	if err != nil {
		return "", err
	}
	input, ok := args["input"].(string)
	if !ok || strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("missing or invalid 'input' argument")
	}

	result, err := textfs.ApplyPatch(store, input)
	if err != nil {
		return "", err
	}
	if result != nil {
		for _, path := range result.Summary.Added {
			notifyMemoryFileChanged(ctx, path)
			maybeRefreshAgentIdentity(ctx, path)
		}
		for _, path := range result.Summary.Modified {
			notifyMemoryFileChanged(ctx, path)
			maybeRefreshAgentIdentity(ctx, path)
		}
		for _, path := range result.Summary.Deleted {
			notifyMemoryFileChanged(ctx, path)
			maybeRefreshAgentIdentity(ctx, path)
		}
		if strings.TrimSpace(result.Text) != "" {
			return result.Text, nil
		}
	}
	return "Patch applied.", nil
}
