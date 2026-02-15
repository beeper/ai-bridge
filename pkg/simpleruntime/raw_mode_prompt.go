package connector

import (
	"strings"
	"time"
)

// buildRawModeSystemPrompt returns the system prompt for raw/playground rooms.
// Raw mode must be simple: a single system prompt with only the current time appended.
func (oc *AIClient) buildRawModeSystemPrompt(meta *PortalMetadata) string {
	base := defaultRawModeSystemPrompt
	if meta != nil {
		if v := strings.TrimSpace(meta.SystemPrompt); v != "" {
			base = v
		}
	}

	_, loc := oc.resolveUserTimezone()
	now := time.Now()
	if loc != nil {
		now = now.In(loc)
	}

	lines := []string{
		strings.TrimSpace(base),
		"Current time: " + now.Format(time.RFC3339),
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
