package codex

import (
	"strings"
)

func aiCapID() string {
	return "com.beeper.ai.v1"
}

func normalizeToolAlias(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}

