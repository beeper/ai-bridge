//lint:file-ignore U1000 Hard-cut compatibility: pending full dead-code deletion.
package connector

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
)

func purgeLoginDataBestEffort(context.Context, *bridgev2.UserLogin) {}

func readStringArgAny(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	raw, ok := args[key]
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func normalizeAgentID(value string) string { return strings.TrimSpace(strings.ToLower(value)) }

func formatCronTime(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func notifyWorkspaceFileChanged(context.Context, string) {}
