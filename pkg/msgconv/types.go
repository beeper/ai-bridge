// Package msgconv provides message conversion utilities for the AI bridge.
// It handles conversion between Matrix message formats and AI provider formats.
package msgconv

import (
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

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
)

// ContentPart represents a single piece of content (text or image)
type ContentPart struct {
	Type     ContentPartType
	Text     string
	ImageURL string // For URL-based images
	ImageB64 string // For base64-encoded images
	MimeType string // e.g., "image/png", "image/jpeg"
}

// ToolCallResult represents the result of a tool call
type ToolCallResult struct {
	ID        string
	Name      string
	Arguments string
	Result    string
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

// hasNonTextParts returns true if parts contain any non-text content
func hasNonTextParts(parts []ContentPart) bool {
	for _, part := range parts {
		if part.Type != ContentTypeText {
			return true
		}
	}
	return false
}

// concatenateText joins all text parts with newlines
func concatenateText(parts []ContentPart) string {
	var texts []string
	for _, part := range parts {
		if part.Type == ContentTypeText && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
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

// ====================
// OpenAI Conversions
// ====================

// buildResponsesContent converts ContentPart slice to Responses API content format
func buildResponsesContent(parts []ContentPart) responses.EasyInputMessageContentUnionParam {
	// If only text parts, use simple string format
	if !hasNonTextParts(parts) {
		return responses.EasyInputMessageContentUnionParam{
			OfString: openai.String(concatenateText(parts)),
		}
	}

	// Build array of content parts
	var contentList responses.ResponseInputMessageContentListParam
	for _, part := range parts {
		switch part.Type {
		case ContentTypeText:
			if part.Text != "" {
				contentList = append(contentList, responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{
						Text: part.Text,
					},
				})
			}
		case ContentTypeImage:
			imageURL := part.ImageURL
			if imageURL == "" && part.ImageB64 != "" {
				// Convert base64 to data URL
				mimeType := part.MimeType
				if mimeType == "" {
					mimeType = "image/png"
				}
				imageURL = "data:" + mimeType + ";base64," + part.ImageB64
			}
			if imageURL != "" {
				contentList = append(contentList, responses.ResponseInputContentUnionParam{
					OfInputImage: &responses.ResponseInputImageParam{
						ImageURL: openai.String(imageURL),
						Detail:   responses.ResponseInputImageDetailAuto,
					},
				})
			}
		}
	}

	return responses.EasyInputMessageContentUnionParam{
		OfInputItemContentList: contentList,
	}
}

// ToOpenAIResponsesInput converts unified messages to OpenAI Responses API format
func ToOpenAIResponsesInput(messages []UnifiedMessage) responses.ResponseInputParam {
	var result responses.ResponseInputParam

	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			result = append(result, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleSystem,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(msg.Text()),
					},
				},
			})
		case RoleUser:
			// User messages can have multimodal content
			result = append(result, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role:    responses.EasyInputMessageRoleUser,
					Content: buildResponsesContent(msg.Content),
				},
			})
		case RoleAssistant:
			result = append(result, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleAssistant,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(msg.Text()),
					},
				},
			})
		case RoleTool:
			// Tool results via function_call_output
			if msg.ToolCallID != "" {
				result = append(result, responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: msg.ToolCallID,
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
							OfString: openai.String(msg.Text()),
						},
					},
				})
			}
		}
	}

	return result
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
