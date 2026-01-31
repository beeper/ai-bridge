// Package aiid provides ID generation and parsing functions for the AI bridge.
// This package contains network ID helpers that follow the mautrix bridgev2 patterns.
package aiid

import (
	"strings"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

// ModelCapabilities stores computed capabilities for a model.
// This is NOT sent to the API, just used for local caching and room features.
type ModelCapabilities struct {
	SupportsVision    bool `json:"supports_vision"`
	SupportsReasoning bool `json:"supports_reasoning"` // Models that support reasoning_effort parameter
	SupportsPDF       bool `json:"supports_pdf"`
	SupportsImageGen  bool `json:"supports_image_gen"`
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
	Capabilities        ModelCapabilities `json:"capabilities,omitzero"`
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
	Model            string `json:"model,omitempty"`
	ReasoningTokens  int64  `json:"reasoning_tokens,omitempty"`
	HasToolCalls     bool   `json:"has_tool_calls,omitempty"`
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
	if src.HasToolCalls {
		mm.HasToolCalls = src.HasToolCalls
	}
}

var _ database.MetaMerger = (*MessageMetadata)(nil)

// Helper functions for extracting typed metadata from bridgev2 objects

// GetPortalMeta extracts PortalMetadata from a portal, creating it if needed
func GetPortalMeta(portal *bridgev2.Portal) *PortalMetadata {
	if portal.Metadata == nil {
		meta := &PortalMetadata{}
		portal.Metadata = meta
		return meta
	}
	if typed, ok := portal.Metadata.(*PortalMetadata); ok {
		return typed
	}
	meta := &PortalMetadata{}
	portal.Metadata = meta
	return meta
}

// GetMessageMeta extracts MessageMetadata from a database message
func GetMessageMeta(msg *database.Message) *MessageMetadata {
	if msg == nil {
		return nil
	}
	if meta, ok := msg.Metadata.(*MessageMetadata); ok {
		return meta
	}
	return nil
}

// ShouldIncludeInHistory checks if a message should be included in LLM history.
// Filters out commands (messages starting with /) and non-conversation messages.
func ShouldIncludeInHistory(meta *MessageMetadata) bool {
	if meta == nil || meta.Body == "" {
		return false
	}
	// Skip command messages
	if strings.HasPrefix(meta.Body, "/") {
		return false
	}
	// Only include user and assistant messages
	if meta.Role != "user" && meta.Role != "assistant" {
		return false
	}
	return true
}
