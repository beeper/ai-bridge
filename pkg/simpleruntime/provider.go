package connector

import (
	"github.com/beeper/ai-bridge/pkg/core/aiprovider"
)

// Type aliases that delegate to pkg/aiprovider.
type (
	AIProvider       = aiprovider.AIProvider
	GenerateParams   = aiprovider.GenerateParams
	GenerateResponse = aiprovider.GenerateResponse
	StreamEventType  = aiprovider.StreamEventType
	StreamEvent      = aiprovider.StreamEvent
	ToolCallResult   = aiprovider.ToolCallResult
	UsageInfo        = aiprovider.UsageInfo
)

// Re-export StreamEventType constants.
const (
	StreamEventDelta     = aiprovider.StreamEventDelta
	StreamEventReasoning = aiprovider.StreamEventReasoning
	StreamEventToolCall  = aiprovider.StreamEventToolCall
	StreamEventComplete  = aiprovider.StreamEventComplete
	StreamEventError     = aiprovider.StreamEventError
)
