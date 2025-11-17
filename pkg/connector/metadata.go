package connector

import "maunium.net/go/mautrix/bridgev2/database"

// UserLoginMetadata is stored on each login row to keep per-user settings.
type UserLoginMetadata struct {
	Persona       string `json:"persona,omitempty"`
	APIKey        string `json:"api_key,omitempty"`
	NextChatIndex int    `json:"next_chat_index,omitempty"`
}

// PortalMetadata stores per-room tuning knobs for the assistant.
type PortalMetadata struct {
	Model               string  `json:"model,omitempty"`
	SystemPrompt        string  `json:"system_prompt,omitempty"`
	Temperature         float64 `json:"temperature,omitempty"`
	MaxContextMessages  int     `json:"max_context_messages,omitempty"`
	MaxCompletionTokens int     `json:"max_completion_tokens,omitempty"`
	Slug                string  `json:"slug,omitempty"`
	Title               string  `json:"title,omitempty"`
	WelcomeSent         bool    `json:"welcome_sent,omitempty"`
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
