package connector

import (
	"strings"
)

// buildRawModeSystemPrompt returns the system prompt for raw/playground rooms.
// Raw mode must be simple: a single system prompt with only time + web_search hints appended.
func (oc *AIClient) buildRawModeSystemPrompt(meta *PortalMetadata) string {
	base := defaultRawModeSystemPrompt
	if meta != nil {
		if v := strings.TrimSpace(meta.SystemPrompt); v != "" {
			base = v
		}
	}

	timezone, _ := oc.resolveUserTimezone()
	now := formatCronTime(timezone)

	lines := []string{
		strings.TrimSpace(base),
		"Current time: " + now,
		"Web search: available via web_search.",
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
