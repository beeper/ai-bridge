package connector

import (
	"maunium.net/go/mautrix/event"
)

// RoomConfigEventType is the Matrix state event type for AI room configuration
var RoomConfigEventType = event.Type{
	Type:  "com.beeper.ai.room_config",
	Class: event.StateEventType,
}

// StreamTokenEventType is the custom event type for streaming token updates
var StreamTokenEventType = event.Type{
	Type:  "com.beeper.ai.stream_token",
	Class: event.MessageEventType,
}

// RoomConfigEventContent represents the content of the room config state event
type RoomConfigEventContent struct {
	Model               string  `json:"model,omitempty"`
	SystemPrompt        string  `json:"system_prompt,omitempty"`
	Temperature         float64 `json:"temperature,omitempty"`
	MaxContextMessages  int     `json:"max_context_messages,omitempty"`
	MaxCompletionTokens int     `json:"max_completion_tokens,omitempty"`
}
