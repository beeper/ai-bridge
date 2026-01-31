package msgconv

import (
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
)

// BuildTextUserMessage creates an OpenAI user message from text
func BuildTextUserMessage(text string) openai.ChatCompletionMessageParamUnion {
	return openai.UserMessage(text)
}

// BuildTextAssistantMessage creates an OpenAI assistant message from text
func BuildTextAssistantMessage(text string) openai.ChatCompletionMessageParamUnion {
	return openai.AssistantMessage(text)
}

// BuildSystemMessage creates an OpenAI system message
func BuildSystemMessage(text string) openai.ChatCompletionMessageParamUnion {
	return openai.SystemMessage(text)
}

// BuildImageUserMessage creates an OpenAI user message with an image
func BuildImageUserMessage(caption, imageURL string) openai.ChatCompletionMessageParamUnion {
	imageContent := openai.ChatCompletionContentPartUnionParam{
		OfImageURL: &openai.ChatCompletionContentPartImageParam{
			ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
				URL:    imageURL,
				Detail: "auto",
			},
		},
	}

	textContent := openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{
			Text: caption,
		},
	}

	return openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
					textContent,
					imageContent,
				},
			},
		},
	}
}

// ConvertMxcToHTTP converts an mxc:// URL to an HTTP URL via the homeserver
func ConvertMxcToHTTP(mxcURL string, homeserver string) string {
	// mxc://server/mediaID -> https://homeserver/_matrix/media/v3/download/server/mediaID
	if !strings.HasPrefix(mxcURL, "mxc://") {
		return mxcURL // Already HTTP
	}

	// Parse mxc URL
	parts := strings.SplitN(strings.TrimPrefix(mxcURL, "mxc://"), "/", 2)
	if len(parts) != 2 {
		return mxcURL
	}

	server := parts[0]
	mediaID := parts[1]

	return fmt.Sprintf("https://%s/_matrix/media/v3/download/%s/%s", homeserver, server, mediaID)
}
