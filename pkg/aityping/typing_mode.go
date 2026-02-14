package aityping

import (
	"strings"
	"time"
)

// TypingMode controls when typing indicators are shown.
type TypingMode string

const (
	TypingModeNever    TypingMode = "never"
	TypingModeInstant  TypingMode = "instant"
	TypingModeThinking TypingMode = "thinking"
	TypingModeMessage  TypingMode = "message"
)

// DefaultTypingInterval is the default interval between typing indicator refreshes.
const DefaultTypingInterval = 6 * time.Second

// NormalizeTypingMode parses a raw string into a known TypingMode.
func NormalizeTypingMode(raw string) (TypingMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "never":
		return TypingModeNever, true
	case "instant":
		return TypingModeInstant, true
	case "thinking":
		return TypingModeThinking, true
	case "message":
		return TypingModeMessage, true
	default:
	}
	return "", false
}
