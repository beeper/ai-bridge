package contracts

// CommonConfig holds cross-bridge defaults shared by all dedicated bridge binaries.
type CommonConfig struct {
	CommandPrefix string
}

// SimpleConfig is the simple bridge-specific config surface.
type SimpleConfig struct {
	EnableWebSearch bool
}

const (
	EventAssistantTurn = "com.beeper.ai.assistant_turn"
	EventStream        = "com.beeper.ai.stream_event"
	EventToolCall      = "com.beeper.ai.tool_call"
	EventToolResult    = "com.beeper.ai.tool_result"
)
