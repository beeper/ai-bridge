package connector

import (
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/beeper/ai-bridge/pkg/aiprovider"
)

// Backward-compatible type aliases that delegate to pkg/aiprovider.
type (
	MessageRole     = aiprovider.MessageRole
	ContentPartType = aiprovider.ContentPartType
	ContentPart     = aiprovider.ContentPart
	UnifiedMessage  = aiprovider.UnifiedMessage
)

// Re-export role constants for backward compatibility.
const (
	RoleSystem    = aiprovider.RoleSystem
	RoleUser      = aiprovider.RoleUser
	RoleAssistant = aiprovider.RoleAssistant
	RoleTool      = aiprovider.RoleTool
)

// Re-export content type constants for backward compatibility.
const (
	ContentTypeText  = aiprovider.ContentTypeText
	ContentTypeImage = aiprovider.ContentTypeImage
	ContentTypePDF   = aiprovider.ContentTypePDF
	ContentTypeAudio = aiprovider.ContentTypeAudio
	ContentTypeVideo = aiprovider.ContentTypeVideo
)

// Re-export constructor functions for backward compatibility.
var (
	NewTextMessage  = aiprovider.NewTextMessage
	NewImageMessage = aiprovider.NewImageMessage
)

// ToOpenAIResponsesInput converts unified messages to OpenAI Responses API format.
// Supports text + image/PDF inputs for user messages; audio/video are intentionally
// excluded (caller should fall back to Chat Completions for those).
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
			var contentParts responses.ResponseInputMessageContentListParam
			hasMultimodal := false
			textContent := ""

			for _, part := range msg.Content {
				switch part.Type {
				case ContentTypeText:
					if strings.TrimSpace(part.Text) == "" {
						continue
					}
					if textContent != "" {
						textContent += "\n"
					}
					textContent += part.Text
				case ContentTypeImage:
					imageURL := strings.TrimSpace(part.ImageURL)
					if imageURL == "" && part.ImageB64 != "" {
						mimeType := part.MimeType
						if mimeType == "" {
							mimeType = "image/jpeg"
						}
						imageURL = buildDataURL(mimeType, part.ImageB64)
					}
					if imageURL == "" {
						continue
					}
					hasMultimodal = true
					contentParts = append(contentParts, responses.ResponseInputContentUnionParam{
						OfInputImage: &responses.ResponseInputImageParam{
							ImageURL: openai.String(imageURL),
							Detail:   responses.ResponseInputImageDetailAuto,
						},
					})
				case ContentTypePDF:
					fileData := strings.TrimSpace(part.PDFB64)
					fileURL := strings.TrimSpace(part.PDFURL)
					if fileData == "" && fileURL == "" {
						continue
					}
					hasMultimodal = true
					fileParam := &responses.ResponseInputFileParam{}
					if fileData != "" {
						fileParam.FileData = openai.String(fileData)
					}
					if fileURL != "" {
						fileParam.FileURL = openai.String(fileURL)
					}
					fileParam.Filename = openai.String("document.pdf")
					contentParts = append(contentParts, responses.ResponseInputContentUnionParam{
						OfInputFile: fileParam,
					})
				case ContentTypeAudio, ContentTypeVideo:
					// Unsupported in Responses API; caller should fall back.
				}
			}

			if textContent != "" {
				textPart := responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{Text: textContent},
				}
				contentParts = append([]responses.ResponseInputContentUnionParam{textPart}, contentParts...)
			}

			if hasMultimodal && len(contentParts) > 0 {
				result = append(result, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfInputItemContentList: contentParts,
						},
					},
				})
			} else if textContent != "" {
				result = append(result, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: openai.String(textContent),
						},
					},
				})
			}
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

// ExtractSystemPrompt extracts the system prompt from unified messages.
// Delegates to aiprovider.ExtractSystemPrompt.
var ExtractSystemPrompt = aiprovider.ExtractSystemPrompt
