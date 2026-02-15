package connector

import "context"

type AIProvider interface {
	Name() string
	GenerateStream(ctx context.Context, params GenerateParams) (<-chan StreamEvent, error)
	Generate(ctx context.Context, params GenerateParams) (*GenerateResponse, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

type GenerateParams struct {
	Model               string
	Messages            []UnifiedMessage
	SystemPrompt        string
	Temperature         float64
	MaxCompletionTokens int
	Tools               []ToolDefinition
	ReasoningEffort     string
	PreviousResponseID  string
	WebSearchEnabled    bool
}

type GenerateResponse struct {
	Content      string
	FinishReason string
	ResponseID   string
	ToolCalls    []ToolCallResult
	Usage        UsageInfo
}

type StreamEventType string

type StreamEvent struct {
	Type           StreamEventType
	Delta          string
	ReasoningDelta string
	ToolCall       *ToolCallResult
	FinishReason   string
	ResponseID     string
	Usage          *UsageInfo
	Error          error
}

type ToolCallResult struct {
	ID        string
	Name      string
	Arguments string
}

type UsageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
}

// Re-export StreamEventType constants.
const (
	StreamEventDelta     StreamEventType = "delta"
	StreamEventReasoning StreamEventType = "reasoning"
	StreamEventToolCall  StreamEventType = "tool_call"
	StreamEventComplete  StreamEventType = "complete"
	StreamEventError     StreamEventType = "error"
)
