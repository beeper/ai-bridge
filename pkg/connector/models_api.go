package connector

import "strings"

// GetModelDisplayName returns the canonical model identifier for display.
func GetModelDisplayName(modelID string) string {
	return ResolveAlias(modelID)
}

// ResolveAlias is intentionally strict in hard-cut mode: only trim whitespace.
func ResolveAlias(modelID string) string {
	return strings.TrimSpace(modelID)
}
