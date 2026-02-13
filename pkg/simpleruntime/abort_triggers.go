package connector

import "strings"

// isAbortTrigger returns true when the message body is a bare "panic button" token.
// This intentionally remains independent of the command system: users can type e.g.
// "stop" to abort an in-flight run.
func isAbortTrigger(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	if trimmed == "" {
		return false
	}
	switch trimmed {
	case "stop", "esc", "abort", "wait", "exit", "interrupt":
		return true
	default:
		return false
	}
}
