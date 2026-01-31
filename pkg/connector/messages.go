package connector

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
	ContentTypePDF   ContentPartType = "pdf"
	ContentTypeAudio ContentPartType = "audio"
	ContentTypeVideo ContentPartType = "video"
)

// ContentPart represents a single piece of content (text, image, PDF, audio, or video)
type ContentPart struct {
	Type     ContentPartType
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
			result = append(result, responses.ResponseInputItemUnionParam{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(msg.Text()),
					},
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
