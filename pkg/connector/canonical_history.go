package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2/database"

	airuntime "github.com/beeper/ai-bridge/pkg/runtime"
)

type canonicalFilePart struct {
	URL       string
	MediaType string
	Filename  string
}

type canonicalToolCall struct {
	callID    string
	toolName  string
	arguments string
}

type canonicalToolOutput struct {
	callID     string
	outputText string
}

func (oc *AIClient) appendHistoryMessageFromCanonical(
	ctx context.Context,
	prompt []openai.ChatCompletionMessageParamUnion,
	_ *database.Message,
	msgMeta *MessageMetadata,
	_ bool,
	injectImages bool,
) []openai.ChatCompletionMessageParamUnion {
	return append(prompt, oc.historyMessageBundle(ctx, msgMeta, injectImages)...)
}

func (oc *AIClient) historyMessageBundle(
	ctx context.Context,
	msgMeta *MessageMetadata,
	injectImages bool,
) []openai.ChatCompletionMessageParamUnion {
	if msgMeta == nil {
		return nil
	}

	role := strings.TrimSpace(msgMeta.Role)
	text := msgMeta.Body
	files := legacyUIMessageFiles(msgMeta)
	toolCalls := legacyToolCalls(msgMeta.ToolCalls)
	toolOutputs := legacyToolOutputs(msgMeta.ToolCalls)
	if len(msgMeta.CanonicalUIMessage) > 0 {
		role = strings.TrimSpace(stringValue(msgMeta.CanonicalUIMessage["role"]))
		text = canonicalUIMessageText(msgMeta.CanonicalUIMessage)
		files = canonicalUIMessageFiles(msgMeta.CanonicalUIMessage)
		toolCalls = canonicalUIToolCalls(msgMeta.CanonicalUIMessage)
		toolOutputs = canonicalUIToolOutputs(msgMeta.CanonicalUIMessage)
	}

	bundle := make([]openai.ChatCompletionMessageParamUnion, 0, 2+len(toolOutputs))
	switch role {
	case "assistant":
		body := airuntime.SanitizeChatMessageForDisplay(stripThinkTags(text), false)
		if assistantMsg, ok := canonicalAssistantHistoryMessage(body, toolCalls); ok {
			bundle = append(bundle, assistantMsg)
		}
		for _, toolOutput := range toolOutputs {
			if toolOutput.callID == "" || toolOutput.outputText == "" {
				continue
			}
			bundle = append(bundle, openai.ToolMessage(toolOutput.outputText, toolOutput.callID))
		}
		if injectImages && len(msgMeta.GeneratedFiles) > 0 {
			if imgParts := oc.downloadGeneratedFileImages(ctx, msgMeta.GeneratedFiles); len(imgParts) > 0 {
				bundle = append(bundle, buildSyntheticGeneratedImagesMessage(msgMeta.GeneratedFiles, imgParts))
			}
		}
	case "user":
		body := airuntime.SanitizeChatMessageForDisplay(text, true)
		if userMsg, ok := oc.canonicalUserHistoryMessage(ctx, body, files, injectImages); ok {
			return append(bundle, userMsg)
		}
		if body != "" {
			bundle = append(bundle, openai.UserMessage(body))
		}
	}
	return bundle
}

func canonicalUIParts(uiMessage map[string]any) []map[string]any {
	switch parts := uiMessage["parts"].(type) {
	case []map[string]any:
		return parts
	case []any:
		out := make([]map[string]any, 0, len(parts))
		for _, raw := range parts {
			part, ok := raw.(map[string]any)
			if ok {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func canonicalUIMessageText(uiMessage map[string]any) string {
	parts := canonicalUIParts(uiMessage)
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(stringValue(part["type"])) != "text" {
			continue
		}
		if text := stringValue(part["text"]); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "")
}

func canonicalUIMessageFiles(uiMessage map[string]any) []canonicalFilePart {
	parts := canonicalUIParts(uiMessage)
	files := make([]canonicalFilePart, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(stringValue(part["type"])) != "file" {
			continue
		}
		url := strings.TrimSpace(stringValue(part["url"]))
		if url == "" {
			continue
		}
		files = append(files, canonicalFilePart{
			URL:       url,
			MediaType: strings.TrimSpace(stringValue(part["mediaType"])),
			Filename:  strings.TrimSpace(stringValue(part["filename"])),
		})
	}
	return files
}

func canonicalUIToolCalls(uiMessage map[string]any) []canonicalToolCall {
	parts := canonicalUIParts(uiMessage)
	toolCalls := make([]canonicalToolCall, 0, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(stringValue(part["type"]))
		if partType != "dynamic-tool" && !strings.HasPrefix(partType, "tool-") {
			continue
		}
		callID := strings.TrimSpace(stringValue(part["toolCallId"]))
		if callID == "" {
			continue
		}
		toolName := strings.TrimSpace(stringValue(part["toolName"]))
		if toolName == "" && strings.HasPrefix(partType, "tool-") {
			toolName = strings.TrimPrefix(partType, "tool-")
		}
		if toolName == "" {
			continue
		}
		toolCalls = append(toolCalls, canonicalToolCall{
			callID:    callID,
			toolName:  toolName,
			arguments: canonicalToolArguments(part["input"]),
		})
	}
	return toolCalls
}

func canonicalUIToolOutputs(uiMessage map[string]any) []canonicalToolOutput {
	parts := canonicalUIParts(uiMessage)
	outputs := make([]canonicalToolOutput, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(stringValue(part["type"])) != "dynamic-tool" {
			continue
		}
		callID := strings.TrimSpace(stringValue(part["toolCallId"]))
		if callID == "" {
			continue
		}
		state := strings.TrimSpace(stringValue(part["state"]))
		switch state {
		case "output-available":
			if text := formatCanonicalValue(part["output"]); text != "" {
				outputs = append(outputs, canonicalToolOutput{callID: callID, outputText: text})
			}
		case "output-error":
			if text := strings.TrimSpace(stringValue(part["errorText"])); text != "" {
				outputs = append(outputs, canonicalToolOutput{callID: callID, outputText: text})
			}
		case "output-denied":
			outputs = append(outputs, canonicalToolOutput{callID: callID, outputText: "Denied by user"})
		}
	}
	return outputs
}

func legacyUIMessageFiles(msgMeta *MessageMetadata) []canonicalFilePart {
	if msgMeta == nil || strings.TrimSpace(msgMeta.MediaURL) == "" {
		return nil
	}
	return []canonicalFilePart{{
		URL:       strings.TrimSpace(msgMeta.MediaURL),
		MediaType: strings.TrimSpace(msgMeta.MimeType),
	}}
}

func legacyToolCalls(toolCalls []ToolCallMetadata) []canonicalToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	out := make([]canonicalToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		callID := strings.TrimSpace(toolCall.CallID)
		toolName := strings.TrimSpace(toolCall.ToolName)
		if callID == "" || toolName == "" {
			continue
		}
		out = append(out, canonicalToolCall{
			callID:    callID,
			toolName:  toolName,
			arguments: canonicalToolArguments(toolCall.Input),
		})
	}
	return out
}

func legacyToolOutputs(toolCalls []ToolCallMetadata) []canonicalToolOutput {
	if len(toolCalls) == 0 {
		return nil
	}
	out := make([]canonicalToolOutput, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		callID := strings.TrimSpace(toolCall.CallID)
		if callID == "" {
			continue
		}
		switch {
		case len(toolCall.Output) > 0:
			if text := formatCanonicalValue(toolCall.Output); text != "" {
				out = append(out, canonicalToolOutput{callID: callID, outputText: text})
			}
		case strings.TrimSpace(toolCall.ErrorMessage) != "":
			out = append(out, canonicalToolOutput{callID: callID, outputText: strings.TrimSpace(toolCall.ErrorMessage)})
		}
	}
	return out
}

func canonicalAssistantHistoryMessage(text string, toolCalls []canonicalToolCall) (openai.ChatCompletionMessageParamUnion, bool) {
	if text == "" && len(toolCalls) == 0 {
		return openai.ChatCompletionMessageParamUnion{}, false
	}

	assistant := openai.ChatCompletionAssistantMessageParam{}
	if text != "" {
		assistant.Content.OfString = openai.String(text)
	}
	if len(toolCalls) > 0 {
		assistant.ToolCalls = make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(toolCalls))
		for _, toolCall := range toolCalls {
			assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: toolCall.callID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      toolCall.toolName,
						Arguments: toolCall.arguments,
					},
					Type: "function",
				},
			})
		}
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant}, true
}

func canonicalToolArguments(raw any) string {
	if value := strings.TrimSpace(formatCanonicalValue(raw)); value != "" {
		return value
	}
	return "{}"
}

func (oc *AIClient) canonicalUserHistoryMessage(
	ctx context.Context,
	body string,
	files []canonicalFilePart,
	injectImages bool,
) (openai.ChatCompletionMessageParamUnion, bool) {
	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(files)+1)
	textWithURLs := body

	for _, file := range files {
		if file.URL == "" {
			continue
		}
		switch {
		case injectImages && isImageMimeType(file.MediaType):
			imgPart := oc.downloadHistoryImage(ctx, file.URL, file.MediaType)
			if imgPart == nil {
				continue
			}
			if textWithURLs != "" {
				textWithURLs += "\n"
			}
			textWithURLs += fmt.Sprintf("[media_url: %s]", file.URL)
			parts = append(parts, *imgPart)
		case strings.HasPrefix(file.MediaType, "audio/"):
			audioPart := oc.downloadHistoryAudio(ctx, file.URL, file.MediaType)
			if audioPart != nil {
				parts = append(parts, *audioPart)
			}
		default:
			filePart := oc.downloadHistoryFile(ctx, file)
			if filePart != nil {
				parts = append(parts, *filePart)
			}
		}
	}

	if textWithURLs != "" {
		parts = append([]openai.ChatCompletionContentPartUnionParam{{
			OfText: &openai.ChatCompletionContentPartTextParam{Text: textWithURLs},
		}}, parts...)
	}
	if len(parts) == 0 {
		return openai.ChatCompletionMessageParamUnion{}, false
	}

	return openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: parts,
			},
		},
	}, true
}

func (oc *AIClient) downloadHistoryFile(ctx context.Context, file canonicalFilePart) *openai.ChatCompletionContentPartUnionParam {
	b64Data, actualMimeType, err := oc.downloadMediaBase64(ctx, file.URL, nil, 50, file.MediaType)
	if err != nil {
		oc.log.Debug().Err(err).Str("url", file.URL).Msg("Failed to download history file, skipping")
		return nil
	}
	fileParam := openai.ChatCompletionContentPartFileFileParam{
		FileData: openai.String(buildDataURL(actualMimeType, b64Data)),
	}
	if file.Filename != "" {
		fileParam.Filename = openai.String(file.Filename)
	}
	return &openai.ChatCompletionContentPartUnionParam{
		OfFile: &openai.ChatCompletionContentPartFileParam{File: fileParam},
	}
}

func (oc *AIClient) downloadHistoryAudio(ctx context.Context, mediaURL, mimeType string) *openai.ChatCompletionContentPartUnionParam {
	b64Data, actualMimeType, err := oc.downloadMediaBase64(ctx, mediaURL, nil, 25, mimeType)
	if err != nil {
		oc.log.Debug().Err(err).Str("url", mediaURL).Msg("Failed to download history audio, skipping")
		return nil
	}
	return &openai.ChatCompletionContentPartUnionParam{
		OfInputAudio: &openai.ChatCompletionContentPartInputAudioParam{
			InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
				Data:   b64Data,
				Format: getAudioFormat(actualMimeType),
			},
		},
	}
}

func formatCanonicalValue(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func stringValue(raw any) string {
	if value, ok := raw.(string); ok {
		return value
	}
	return ""
}
