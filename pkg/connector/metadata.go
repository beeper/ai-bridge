package connector

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/bridgev2/database"
)

// ModelCache stores available models (cached in UserLoginMetadata)
// Uses provider-agnostic ModelInfo instead of openai.Model
type ModelCache struct {
	Models        []ModelInfo `json:"models,omitempty"`
	LastRefresh   int64       `json:"last_refresh,omitempty"`
	CacheDuration int64       `json:"cache_duration,omitempty"` // seconds
}

// ModelCapabilities stores computed capabilities for a model
// This is NOT sent to the API, just used for local caching
type ModelCapabilities struct {
	SupportsVision    bool `json:"supports_vision"`
	SupportsReasoning bool `json:"supports_reasoning"` // Models that support reasoning_effort parameter
	SupportsPDF       bool `json:"supports_pdf"`
	SupportsImageGen  bool `json:"supports_image_gen"`
	SupportsAudio     bool `json:"supports_audio"` // Models that accept audio input
	SupportsVideo     bool `json:"supports_video"` // Models that accept video input
}

// UserLoginMetadata is stored on each login row to keep per-user settings.
type UserLoginMetadata struct {
	Persona       string      `json:"persona,omitempty"`
	Provider      string      `json:"provider,omitempty"` // Selected provider (beeper, openai, openrouter, custom)
	APIKey        string      `json:"api_key,omitempty"`
	BaseURL       string      `json:"base_url,omitempty"` // Per-user API endpoint
	NextChatIndex int         `json:"next_chat_index,omitempty"`
	ModelCache    *ModelCache `json:"model_cache,omitempty"`
}

// PortalMetadata stores per-room tuning knobs for the assistant.
type PortalMetadata struct {
	Model               string            `json:"model,omitempty"`                 // Set from room state
	SystemPrompt        string            `json:"system_prompt,omitempty"`         // Set from room state
	Temperature         float64           `json:"temperature,omitempty"`           // Set from room state
	MaxContextMessages  int               `json:"max_context_messages,omitempty"`  // Set from room state
	MaxCompletionTokens int               `json:"max_completion_tokens,omitempty"` // Set from room state
	ReasoningEffort     string            `json:"reasoning_effort,omitempty"`      // none, low, medium, high, xhigh
	Slug                string            `json:"slug,omitempty"`
	Title               string            `json:"title,omitempty"`
	TitleGenerated      bool              `json:"title_generated,omitempty"` // True if title was auto-generated
	WelcomeSent         bool              `json:"welcome_sent,omitempty"`
	Capabilities        ModelCapabilities `json:"capabilities,omitempty"`
	LastRoomStateSync   int64             `json:"last_room_state_sync,omitempty"` // Track when we've synced room state
	ToolsEnabled        bool              `json:"tools_enabled,omitempty"`        // Enable function calling tools

	// Conversation context mode: "messages" (build full history) or "responses" (use previous_response_id)
	ConversationMode string `json:"conversation_mode,omitempty"` // Default: "messages"
	LastResponseID   string `json:"last_response_id,omitempty"`  // For "responses" mode - chain responses

	// Responses API built-in tools
	WebSearchEnabled       bool `json:"web_search_enabled,omitempty"`
	FileSearchEnabled      bool `json:"file_search_enabled,omitempty"`
	CodeInterpreterEnabled bool `json:"code_interpreter_enabled,omitempty"`

	// Streaming configuration
	EmitThinking bool `json:"emit_thinking,omitempty"` // Stream thinking/reasoning tokens
	EmitToolArgs bool `json:"emit_tool_args,omitempty"` // Stream tool argument tokens

	// Agent configuration (for multi-agent rooms)
	DefaultAgentID string `json:"default_agent_id,omitempty"` // Which agent responds by default
}

// MessageMetadata keeps a tiny summary of each exchange so we can rebuild
// prompts using database history.
type MessageMetadata struct {
	Role             string `json:"role,omitempty"`
	Body             string `json:"body,omitempty"`
	CompletionID     string `json:"completion_id,omitempty"`
	FinishReason     string `json:"finish_reason,omitempty"`
	PromptTokens     int64  `json:"prompt_tokens,omitempty"`
	CompletionTokens int64  `json:"completion_tokens,omitempty"`
	Model            string `json:"model,omitempty"`
	ReasoningTokens  int64  `json:"reasoning_tokens,omitempty"`
	HasToolCalls     bool   `json:"has_tool_calls,omitempty"`

	// Turn tracking for the new schema
	TurnID  string `json:"turn_id,omitempty"`  // Unique identifier for this assistant turn
	AgentID string `json:"agent_id,omitempty"` // Which agent generated this (for multi-agent rooms)

	// Tool call tracking
	ToolCalls []ToolCallMetadata `json:"tool_calls,omitempty"` // List of tool calls in this turn

	// Timing information
	StartedAtMs    int64 `json:"started_at_ms,omitempty"`    // Unix ms when generation started
	FirstTokenAtMs int64 `json:"first_token_at_ms,omitempty"` // Unix ms of first token
	CompletedAtMs  int64 `json:"completed_at_ms,omitempty"`   // Unix ms when completed

	// Thinking/reasoning content (embedded, not separate)
	ThinkingContent    string `json:"thinking_content,omitempty"`     // Full thinking text
	ThinkingTokenCount int    `json:"thinking_token_count,omitempty"` // Number of thinking tokens
}

// ToolCallMetadata tracks a tool call within a message
type ToolCallMetadata struct {
	CallID       string         `json:"call_id"`
	ToolName     string         `json:"tool_name"`
	ToolType     string         `json:"tool_type"` // builtin, provider, function, mcp
	Input        map[string]any `json:"input,omitempty"`
	Output       map[string]any `json:"output,omitempty"`
	Status       string         `json:"status"`                  // pending, running, completed, failed, timeout, cancelled
	ResultStatus string         `json:"result_status,omitempty"` // success, error, partial
	ErrorMessage string         `json:"error_message,omitempty"`
	StartedAtMs  int64          `json:"started_at_ms,omitempty"`
	CompletedAtMs int64         `json:"completed_at_ms,omitempty"`

	// Event IDs for timeline events (if emitted as separate events)
	CallEventID   string `json:"call_event_id,omitempty"`
	ResultEventID string `json:"result_event_id,omitempty"`
}

// GhostMetadata stores metadata for AI model ghosts
type GhostMetadata struct {
	LastSync jsontime.Unix `json:"last_sync,omitempty"`
}

// CopyFrom allows the metadata struct to participate in mautrix's meta merge.
func (mm *MessageMetadata) CopyFrom(other any) {
	src, ok := other.(*MessageMetadata)
	if !ok || src == nil {
		return
	}
	if src.Role != "" {
		mm.Role = src.Role
	}
	if src.Body != "" {
		mm.Body = src.Body
	}
	if src.CompletionID != "" {
		mm.CompletionID = src.CompletionID
	}
	if src.FinishReason != "" {
		mm.FinishReason = src.FinishReason
	}
	if src.PromptTokens != 0 {
		mm.PromptTokens = src.PromptTokens
	}
	if src.CompletionTokens != 0 {
		mm.CompletionTokens = src.CompletionTokens
	}
	if src.Model != "" {
		mm.Model = src.Model
	}
	if src.ReasoningTokens != 0 {
		mm.ReasoningTokens = src.ReasoningTokens
	}
	mm.HasToolCalls = src.HasToolCalls

	// Copy new fields
	if src.TurnID != "" {
		mm.TurnID = src.TurnID
	}
	if src.AgentID != "" {
		mm.AgentID = src.AgentID
	}
	if len(src.ToolCalls) > 0 {
		mm.ToolCalls = src.ToolCalls
	}
	if src.StartedAtMs != 0 {
		mm.StartedAtMs = src.StartedAtMs
	}
	if src.FirstTokenAtMs != 0 {
		mm.FirstTokenAtMs = src.FirstTokenAtMs
	}
	if src.CompletedAtMs != 0 {
		mm.CompletedAtMs = src.CompletedAtMs
	}
	if src.ThinkingContent != "" {
		mm.ThinkingContent = src.ThinkingContent
	}
	if src.ThinkingTokenCount != 0 {
		mm.ThinkingTokenCount = src.ThinkingTokenCount
	}
}

var _ database.MetaMerger = (*MessageMetadata)(nil)

// NewTurnID generates a new unique turn ID
func NewTurnID() string {
	// Use a simple timestamp-based ID for now
	// Could be enhanced with UUID or other unique ID generation
	return "turn_" + generateShortID()
}

// NewCallID generates a new unique call ID for tool calls
func NewCallID() string {
	return "call_" + generateShortID()
}

// generateShortID generates a short unique ID (12 chars) using crypto/rand
func generateShortID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().Format("060102150405")))[:12]
	}
	return hex.EncodeToString(b)
}
