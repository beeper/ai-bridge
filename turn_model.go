package agentremote

import "github.com/beeper/agentremote/turns"

// AgentMessageRole is the canonical internal role for Pi-style turn messages.
type AgentMessageRole string

const (
	RoleAssistant    AgentMessageRole = "assistant"
	RoleUser         AgentMessageRole = "user"
	RoleToolResult   AgentMessageRole = "tool_result"
	RoleNotification AgentMessageRole = "notification"
	RoleProgress     AgentMessageRole = "progress"
)

// AgentMessage is the internal turn-native message representation used by the
// public agentremote runtime. Matrix/AI SDK payloads are derived projections.
type AgentMessage struct {
	ID        string
	Role      AgentMessageRole
	Text      string
	Metadata  map[string]any
	Timestamp int64
}

// ToolExecutionState tracks the lifecycle of a tool call within a turn.
type ToolExecutionState struct {
	CallID        string
	ToolName      string
	Status        string
	Args          map[string]any
	Result        map[string]any
	PartialResult map[string]any
	IsError       bool
}

// TurnEventType enumerates the canonical internal turn lifecycle.
type TurnEventType string

const (
	TurnEventStart                 TurnEventType = "turn_start"
	TurnEventMessageStart          TurnEventType = "message_start"
	TurnEventMessageUpdate         TurnEventType = "message_update"
	TurnEventMessageEnd            TurnEventType = "message_end"
	TurnEventToolExecutionStart    TurnEventType = "tool_execution_start"
	TurnEventToolExecutionUpdate   TurnEventType = "tool_execution_update"
	TurnEventToolExecutionApproval TurnEventType = "tool_execution_approval_required"
	TurnEventToolExecutionEnd      TurnEventType = "tool_execution_end"
	TurnEventEnd                   TurnEventType = "turn_end"
	TurnEventAbort                 TurnEventType = "turn_abort"
	TurnEventError                 TurnEventType = "turn_error"
)

// TurnEvent is the canonical internal event emitted by a managed turn.
type TurnEvent struct {
	Type          TurnEventType
	Message       *AgentMessage
	ToolExecution *ToolExecutionState
	Error         string
	Metadata      map[string]any
	Timestamp     int64
}

// TurnSnapshot is the durable in-memory representation of a turn as events are
// applied. Bridges can project this state into Matrix/Beeper payloads.
type TurnSnapshot struct {
	TurnID           string
	AgentID          string
	VisibleText      string
	ReasoningText    string
	Messages         []AgentMessage
	ToolExecutions   []ToolExecutionState
	Events           []TurnEvent
	StartedAtMs      int64
	FirstTokenAtMs   int64
	CompletedAtMs    int64
	FinishReason     string
	LastError        string
	NetworkMessageID string
	TargetEventID    string
}

// TurnManager tracks active turns for a runtime.
type TurnManager struct{}

// TurnOptions configures a new managed turn.
type TurnOptions struct {
	ID      string
	AgentID string
}

// Turn is the public managed turn handle. It owns the Pi-style snapshot and can
// optionally attach to a streaming transport session.
type Turn struct {
	ID      string
	AgentID string

	Snapshot TurnSnapshot
	Session  *turns.StreamSession
}
