package opencodebridge

import (
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/ai-bridge/pkg/bridgeadapter"
)

type MessageMetadata struct {
	Role               string                           `json:"role,omitempty"`
	Body               string                           `json:"body,omitempty"`
	SessionID          string                           `json:"session_id,omitempty"`
	MessageID          string                           `json:"message_id,omitempty"`
	ParentMessageID    string                           `json:"parent_message_id,omitempty"`
	Agent              string                           `json:"agent,omitempty"`
	ModelID            string                           `json:"model_id,omitempty"`
	ProviderID         string                           `json:"provider_id,omitempty"`
	Mode               string                           `json:"mode,omitempty"`
	FinishReason       string                           `json:"finish_reason,omitempty"`
	ErrorText          string                           `json:"error_text,omitempty"`
	Cost               float64                          `json:"cost,omitempty"`
	PromptTokens       int64                            `json:"prompt_tokens,omitempty"`
	CompletionTokens   int64                            `json:"completion_tokens,omitempty"`
	ReasoningTokens    int64                            `json:"reasoning_tokens,omitempty"`
	TotalTokens        int64                            `json:"total_tokens,omitempty"`
	TurnID             string                           `json:"turn_id,omitempty"`
	AgentID            string                           `json:"agent_id,omitempty"`
	CanonicalSchema    string                           `json:"canonical_schema,omitempty"`
	CanonicalUIMessage map[string]any                   `json:"canonical_ui_message,omitempty"`
	StartedAtMs        int64                            `json:"started_at_ms,omitempty"`
	CompletedAtMs      int64                            `json:"completed_at_ms,omitempty"`
	ThinkingContent    string                           `json:"thinking_content,omitempty"`
	ToolCalls          []bridgeadapter.ToolCallMetadata `json:"tool_calls,omitempty"`
	GeneratedFiles     []bridgeadapter.GeneratedFileRef `json:"generated_files,omitempty"`
}

type ToolCallMetadata = bridgeadapter.ToolCallMetadata

type GeneratedFileRef = bridgeadapter.GeneratedFileRef

var _ database.MetaMerger = (*MessageMetadata)(nil)

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
	if src.SessionID != "" {
		mm.SessionID = src.SessionID
	}
	if src.MessageID != "" {
		mm.MessageID = src.MessageID
	}
	if src.ParentMessageID != "" {
		mm.ParentMessageID = src.ParentMessageID
	}
	if src.Agent != "" {
		mm.Agent = src.Agent
	}
	if src.ModelID != "" {
		mm.ModelID = src.ModelID
	}
	if src.ProviderID != "" {
		mm.ProviderID = src.ProviderID
	}
	if src.Mode != "" {
		mm.Mode = src.Mode
	}
	if src.FinishReason != "" {
		mm.FinishReason = src.FinishReason
	}
	if src.ErrorText != "" {
		mm.ErrorText = src.ErrorText
	}
	if src.Cost != 0 {
		mm.Cost = src.Cost
	}
	if src.PromptTokens != 0 {
		mm.PromptTokens = src.PromptTokens
	}
	if src.CompletionTokens != 0 {
		mm.CompletionTokens = src.CompletionTokens
	}
	if src.ReasoningTokens != 0 {
		mm.ReasoningTokens = src.ReasoningTokens
	}
	if src.TotalTokens != 0 {
		mm.TotalTokens = src.TotalTokens
	}
	if src.TurnID != "" {
		mm.TurnID = src.TurnID
	}
	if src.AgentID != "" {
		mm.AgentID = src.AgentID
	}
	if src.CanonicalSchema != "" {
		mm.CanonicalSchema = src.CanonicalSchema
	}
	if len(src.CanonicalUIMessage) > 0 {
		mm.CanonicalUIMessage = src.CanonicalUIMessage
	}
	if src.StartedAtMs != 0 {
		mm.StartedAtMs = src.StartedAtMs
	}
	if src.CompletedAtMs != 0 {
		mm.CompletedAtMs = src.CompletedAtMs
	}
	if src.ThinkingContent != "" {
		mm.ThinkingContent = src.ThinkingContent
	}
	if len(src.ToolCalls) > 0 {
		mm.ToolCalls = src.ToolCalls
	}
	if len(src.GeneratedFiles) > 0 {
		mm.GeneratedFiles = src.GeneratedFiles
	}
}
