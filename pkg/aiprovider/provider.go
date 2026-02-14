package aiprovider

import (
	"context"
	"strings"

	"github.com/beeper/ai-bridge/pkg/aimodels"
)

// AIProvider defines a common interface for OpenAI-compatible AI providers
type AIProvider interface {
	// Name returns the provider name (e.g., "openai", "openrouter")
	Name() string

	// GenerateStream generates a streaming response
	GenerateStream(ctx context.Context, params GenerateParams) (<-chan StreamEvent, error)

	// Generate generates a non-streaming response
	Generate(ctx context.Context, params GenerateParams) (*GenerateResponse, error)

	// ListModels returns available models for this provider
	ListModels(ctx context.Context) ([]aimodels.ModelInfo, error)
}

// GenerateParams contains parameters for generation requests
type GenerateParams struct {
	Model               string
	Messages            []UnifiedMessage
	SystemPrompt        string
	Temperature         float64
	MaxCompletionTokens int
	Tools               []ToolDefinition
	ReasoningEffort     string // none, low, medium, high (for reasoning models)

	// Responses API specific
	PreviousResponseID string // For conversation continuation
	WebSearchEnabled   bool
}

// GenerateResponse contains the result of a non-streaming generation
type GenerateResponse struct {
	Content      string
	FinishReason string
	ResponseID   string // For Responses API continuation
	ToolCalls    []ToolCallResult
	Usage        UsageInfo
}

// StreamEventType identifies the type of streaming event
type StreamEventType string

const (
	StreamEventDelta     StreamEventType = "delta"     // Text content delta
	StreamEventReasoning StreamEventType = "reasoning" // Reasoning/thinking delta
	StreamEventToolCall  StreamEventType = "tool_call" // Tool call request
	StreamEventComplete  StreamEventType = "complete"  // Generation complete
	StreamEventError     StreamEventType = "error"     // Error occurred
)

// StreamEvent represents a single event from a streaming response
type StreamEvent struct {
	Type           StreamEventType
	Delta          string          // Text chunk for delta events
	ReasoningDelta string          // Thinking/reasoning chunk
	ToolCall       *ToolCallResult // For tool_call events
	FinishReason   string          // For complete events
	ResponseID     string          // Response ID (for Responses API)
	Usage          *UsageInfo      // Token usage (usually on complete)
	Error          error           // For error events
}

// ToolCallResult represents a tool/function call from the model
type ToolCallResult struct {
	ID        string
	Name      string
	Arguments string // JSON string of arguments
}

// UsageInfo contains token usage information
type UsageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int // For models with extended thinking
}

// MessageRole represents the role of a message sender
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// ContentPartType identifies the type of content in a message
type ContentPartType string

const (
	ContentTypeText  ContentPartType = "text"
	ContentTypeImage ContentPartType = "image"
	ContentTypePDF   ContentPartType = "pdf"
	ContentTypeAudio ContentPartType = "audio"
	ContentTypeVideo ContentPartType = "video"
)

// ContentPart represents a single piece of content (text, image, PDF, audio, or video)
type ContentPart struct {
	Type        ContentPartType
	Text        string
	ImageURL    string
	ImageB64    string
	MimeType    string
	PDFURL      string
	PDFB64      string
	AudioB64    string
	AudioFormat string // wav, mp3, webm, ogg, flac
	VideoURL    string
	VideoB64    string
}

// UnifiedMessage is a provider-agnostic message format
type UnifiedMessage struct {
	Role       MessageRole
	Content    []ContentPart
	ToolCalls  []ToolCallResult // For assistant messages with tool calls
	ToolCallID string           // For tool result messages
	Name       string           // Optional name for the message sender
}

// Text returns the text content of a message (concatenating all text parts)
func (m *UnifiedMessage) Text() string {
	var texts []string
	for _, part := range m.Content {
		if part.Type == ContentTypeText {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// HasImages returns true if the message contains image content
func (m *UnifiedMessage) HasImages() bool {
	for _, part := range m.Content {
		if part.Type == ContentTypeImage {
			return true
		}
	}
	return false
}

// HasMultimodalContent returns true if the message contains any non-text content
func (m *UnifiedMessage) HasMultimodalContent() bool {
	for _, part := range m.Content {
		switch part.Type {
		case ContentTypeImage, ContentTypePDF, ContentTypeAudio, ContentTypeVideo:
			return true
		}
	}
	return false
}

// NewTextMessage creates a simple text message
func NewTextMessage(role MessageRole, text string) UnifiedMessage {
	return UnifiedMessage{
		Role: role,
		Content: []ContentPart{
			{Type: ContentTypeText, Text: text},
		},
	}
}

// NewImageMessage creates a message with an image
func NewImageMessage(role MessageRole, imageURL, mimeType string) UnifiedMessage {
	return UnifiedMessage{
		Role: role,
		Content: []ContentPart{
			{Type: ContentTypeImage, ImageURL: imageURL, MimeType: mimeType},
		},
	}
}

// ExtractSystemPrompt extracts the system prompt from unified messages
func ExtractSystemPrompt(messages []UnifiedMessage) string {
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			return msg.Text()
		}
	}
	return ""
}

// ToolDefinition defines a tool callable by the model (data fields only).
// The connector package extends this with an Execute function.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}
