package agents

import "errors"

// Agent-related errors.
var (
	ErrMissingAgentID   = errors.New("agent ID is required")
	ErrMissingAgentName = errors.New("agent name is required")
	ErrAgentNotFound    = errors.New("agent not found")
	ErrAgentIsPreset    = errors.New("cannot modify preset agent")
	ErrDuplicateAgentID = errors.New("agent ID already exists")
)

// Context guard errors.
var (
	ErrContextOverflow   = errors.New("context limit exceeded")
	ErrHighTurnRate      = errors.New("too many messages in short period")
	ErrTokenLimitReached = errors.New("estimated token limit reached")
)
