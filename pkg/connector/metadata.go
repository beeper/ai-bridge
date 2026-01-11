package connector

import (
	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2/database"
)

// ModelCache stores available models (cached in UserLoginMetadata)
// Uses openai.Model directly - no need to reinvent the struct
type ModelCache struct {
	Models        []openai.Model `json:"models,omitempty"`
	LastRefresh   int64          `json:"last_refresh,omitempty"`
	CacheDuration int64          `json:"cache_duration,omitempty"` // seconds
}

// ModelCapabilities stores computed capabilities for a model
// This is NOT sent to the API, just used for local caching
type ModelCapabilities struct {
	SupportsVision   bool `json:"supports_vision"`
	IsReasoningModel bool `json:"is_reasoning_model"` // O1/O3 models - no streaming, longer timeouts
}

// UserLoginMetadata is stored on each login row to keep per-user settings.
type UserLoginMetadata struct {
	Persona       string      `json:"persona,omitempty"`
	Provider      string      `json:"provider,omitempty"`            // Selected provider (beeper, openai, openrouter, custom)
	APIKey        string      `json:"api_key,omitempty"`
	BaseURL       string      `json:"base_url,omitempty"`            // Per-user API endpoint
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
}

var _ database.MetaMerger = (*MessageMetadata)(nil)
