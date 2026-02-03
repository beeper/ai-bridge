package connector

import (
	"context"
	"fmt"

	"github.com/beeper/ai-bridge/pkg/textfs"
)

func executeApplyPatch(ctx context.Context, args map[string]any) (string, error) {
	store, err := textFSStore(ctx)
	if err != nil {
		return "", err
	}
	input, ok := readStringArg(args, "input", "patch")
	if !ok {
		return "", fmt.Errorf("missing or invalid 'input' argument")
	}
	result, err := textfs.ApplyPatch(store, input)
	if err != nil {
		return "", err
	}
	if result != nil {
		for _, path := range result.Summary.Added {
			notifyMemoryFileChanged(ctx, path)
		}
		for _, path := range result.Summary.Modified {
			notifyMemoryFileChanged(ctx, path)
		}
		for _, path := range result.Summary.Deleted {
			notifyMemoryFileChanged(ctx, path)
		}
		return result.Text, nil
	}
	return "", nil
}
