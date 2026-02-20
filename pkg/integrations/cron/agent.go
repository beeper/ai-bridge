package cron

import "strings"

// ResolveCronAgentID normalizes and validates a cron agent id.
func ResolveCronAgentID(raw string, defaultAgentID string, normalize func(string) string, isKnown func(string) bool) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "main") {
		return defaultAgentID
	}
	if normalize == nil {
		normalize = func(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
	}
	normalized := normalize(trimmed)
	if isKnown != nil && isKnown(normalized) {
		return normalized
	}
	return defaultAgentID
}
