package connector

import (
	"strings"

	"github.com/beeper/ai-bridge/pkg/agents"
)

func resolveCronAgentID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "main") {
		return agents.DefaultAgentID
	}
	return trimmed
}
