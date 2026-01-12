package connector

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"
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

// ToOpenAIChatMessages converts unified messages to OpenAI Chat Completions format
func ToOpenAIChatMessages(messages []UnifiedMessage) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion

	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			result = append(result, openai.SystemMessage(msg.Text()))
		case RoleUser:
			if msg.HasImages() {
				// Multi-part user message with images
				var parts []openai.ChatCompletionContentPartUnionParam
				for _, content := range msg.Content {
					switch content.Type {
					case ContentTypeText:
						parts = append(parts, openai.ChatCompletionContentPartUnionParam{
							OfText: &openai.ChatCompletionContentPartTextParam{
								Text: content.Text,
							},
						})
					case ContentTypeImage:
						imageURL := content.ImageURL
						if content.ImageB64 != "" {
							imageURL = "data:" + content.MimeType + ";base64," + content.ImageB64
						}
						if imageURL != "" {
							parts = append(parts, openai.ChatCompletionContentPartUnionParam{
								OfImageURL: &openai.ChatCompletionContentPartImageParam{
									ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
										URL: imageURL,
									},
								},
							})
						}
					}
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfArrayOfContentParts: parts,
						},
					},
				})
			} else {
				result = append(result, openai.UserMessage(msg.Text()))
			}
		case RoleAssistant:
			result = append(result, openai.AssistantMessage(msg.Text()))
		case RoleTool:
			if msg.ToolCallID != "" {
				result = append(result, openai.ToolMessage(msg.ToolCallID, msg.Text()))
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

// ====================
// Gemini Conversions
// ====================

// ToGeminiContents converts unified messages to Gemini format
func ToGeminiContents(messages []UnifiedMessage) []*genai.Content {
	var result []*genai.Content

	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			// System prompts handled via SystemInstruction
			continue
		case RoleUser:
			content := &genai.Content{Role: "user"}
			for _, part := range msg.Content {
				switch part.Type {
				case ContentTypeText:
					content.Parts = append(content.Parts, &genai.Part{Text: part.Text})
				case ContentTypeImage:
					if part.ImageB64 != "" {
						data, _ := base64.StdEncoding.DecodeString(part.ImageB64)
						content.Parts = append(content.Parts, &genai.Part{
							InlineData: &genai.Blob{
								MIMEType: part.MimeType,
								Data:     data,
							},
						})
					}
				}
			}
			result = append(result, content)
		case RoleAssistant:
			content := &genai.Content{Role: "model"}
			content.Parts = append(content.Parts, &genai.Part{Text: msg.Text()})
			// Handle tool calls
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				json.Unmarshal([]byte(tc.Arguments), &args)
				content.Parts = append(content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name: tc.Name,
						Args: args,
					},
				})
			}
			result = append(result, content)
		case RoleTool:
			// Tool results in Gemini
			content := &genai.Content{Role: "user"}
			var response map[string]any
			json.Unmarshal([]byte(msg.Text()), &response)
			if response == nil {
				response = map[string]any{"result": msg.Text()}
			}
			content.Parts = append(content.Parts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     msg.Name,
					Response: response,
				},
			})
			result = append(result, content)
		}
	}

	return result
}

// ====================
// Anthropic Conversions
// ====================

// ToAnthropicMessages converts unified messages to Anthropic format
func ToAnthropicMessages(messages []UnifiedMessage) []anthropic.MessageParam {
	var result []anthropic.MessageParam

	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			// System prompts handled separately
			continue
		case RoleUser:
			var blocks []anthropic.ContentBlockParamUnion
			for _, part := range msg.Content {
				switch part.Type {
				case ContentTypeText:
					blocks = append(blocks, anthropic.NewTextBlock(part.Text))
				case ContentTypeImage:
					if part.ImageB64 != "" {
						blocks = append(blocks, anthropic.NewImageBlockBase64(part.MimeType, part.ImageB64))
					} else if part.ImageURL != "" {
						blocks = append(blocks, anthropic.NewImageBlock(anthropic.URLImageSourceParam{
							URL: part.ImageURL,
						}))
					}
				}
			}
			// Add tool results if this is a tool response message
			if msg.ToolCallID != "" {
				blocks = append(blocks, anthropic.NewToolResultBlock(msg.ToolCallID, msg.Text(), false))
			}
			if len(blocks) > 0 {
				result = append(result, anthropic.NewUserMessage(blocks...))
			}
		case RoleAssistant:
			var blocks []anthropic.ContentBlockParamUnion
			if text := msg.Text(); text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(text))
			}
			// Handle tool calls
			for _, tc := range msg.ToolCalls {
				var input any
				json.Unmarshal([]byte(tc.Arguments), &input)
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
			}
			if len(blocks) > 0 {
				result = append(result, anthropic.NewAssistantMessage(blocks...))
			}
		case RoleTool:
			// Tool results are added as user messages in Anthropic
			result = append(result, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(msg.ToolCallID, msg.Text(), false),
			))
		}
	}

	return result
}

// ToAnthropicSystemPrompt converts system prompt to Anthropic format
func ToAnthropicSystemPrompt(systemPrompt string) []anthropic.TextBlockParam {
	if systemPrompt == "" {
		return nil
	}
	return []anthropic.TextBlockParam{
		{Text: systemPrompt},
	}
}
