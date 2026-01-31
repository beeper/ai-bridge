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

// StreamContentType identifies the type of content in a stream delta
type StreamContentType string

const (
	StreamContentText       StreamContentType = "text"
	StreamContentReasoning  StreamContentType = "reasoning"
	StreamContentToolCall   StreamContentType = "tool_call"
	StreamContentToolResult StreamContentType = "tool_result"
	StreamContentImage      StreamContentType = "image"
)

// RoomConfigEventContent represents the content of the room config state event
type RoomConfigEventContent struct {
	Model               string  `json:"model,omitempty"`
	SystemPrompt        string  `json:"system_prompt,omitempty"`
	Temperature         float64 `json:"temperature,omitempty"`
	MaxContextMessages  int     `json:"max_context_messages,omitempty"`
	MaxCompletionTokens int     `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string  `json:"reasoning_effort,omitempty"`
	ToolsEnabled        bool    `json:"tools_enabled,omitempty"`
	ConversationMode    string  `json:"conversation_mode,omitempty"` // "messages" or "responses"

	// Responses API built-in tools
	WebSearchEnabled       bool `json:"web_search_enabled,omitempty"`
	FileSearchEnabled      bool `json:"file_search_enabled,omitempty"`
	CodeInterpreterEnabled bool `json:"code_interpreter_enabled,omitempty"`
}

// ModelCapabilitiesEventType is the Matrix state event type for broadcasting available models
var ModelCapabilitiesEventType = event.Type{
	Type:  "com.beeper.ai.model_capabilities",
	Class: event.StateEventType,
}

// ModelCapabilitiesEventContent represents available models and their capabilities
type ModelCapabilitiesEventContent struct {
	AvailableModels []ModelInfo `json:"available_models"`
}

// Tool constants for model capabilities
const (
	ToolWebSearch       = "web_search"
	ToolFunctionCalling = "function_calling"
	ToolCodeInterpreter = "code_interpreter"
)

// ModelInfo describes a single AI model's capabilities
type ModelInfo struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Provider            string   `json:"provider"`
	Description         string   `json:"description,omitempty"`
	SupportsVision      bool     `json:"supports_vision"`
	SupportsToolCalling bool     `json:"supports_tool_calling"`
	SupportsPDF         bool     `json:"supports_pdf,omitempty"`
	SupportsReasoning   bool     `json:"supports_reasoning"`  // Model can use reasoning when enabled
	SupportsWebSearch   bool     `json:"supports_web_search"` // Model supports web search plugin
	SupportsImageGen    bool     `json:"supports_image_gen,omitempty"`
	ContextWindow       int      `json:"context_window,omitempty"`
	MaxOutputTokens     int      `json:"max_output_tokens,omitempty"`
	AvailableTools      []string `json:"available_tools,omitempty"` // List of supported tools
}
