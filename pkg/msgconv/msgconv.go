package msgconv

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
)

// MessageConverter handles conversion between Matrix and AI message formats
type MessageConverter struct {
	// MaxImageSize is the maximum allowed image size in bytes
	MaxImageSize int64
	// MaxCaptionLength is the maximum caption length
	MaxCaptionLength int
}

// NewMessageConverter creates a new message converter with default settings
func NewMessageConverter() *MessageConverter {
	return &MessageConverter{
		MaxImageSize:     20 * 1024 * 1024, // 20MB
		MaxCaptionLength: 100000,
	}
}

// ConvertContext provides context for message conversion operations
type ConvertContext struct {
	Ctx    context.Context
	Portal *bridgev2.Portal
	Intent bridgev2.MatrixAPI
}

// NewConvertContext creates a new conversion context
func NewConvertContext(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI) *ConvertContext {
	return &ConvertContext{
		Ctx:    ctx,
		Portal: portal,
		Intent: intent,
	}
}
