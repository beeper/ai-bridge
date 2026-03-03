package connector

import "strings"

// ResolveAlias normalizes a model identifier by trimming whitespace.
func ResolveAlias(modelID string) string {
	return strings.TrimSpace(modelID)
}
