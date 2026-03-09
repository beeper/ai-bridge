package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

func (oc *AIClient) historyMessageBundle(
	ctx context.Context,
	msgMeta *MessageMetadata,
	injectImages bool,
) []PromptMessage {
	if msgMeta == nil {
		return nil
	}
	if canonical := filterPromptMessagesForHistory(canonicalPromptMessages(msgMeta), injectImages); len(canonical) > 0 {
		if injectImages && len(msgMeta.GeneratedFiles) > 0 {
			if generated := oc.generatedImagesHistoryMessage(ctx, msgMeta.GeneratedFiles); len(generated.Blocks) > 0 {
				return append(canonical, generated)
			}
		}
		return canonical
	}

	role := strings.TrimSpace(msgMeta.Role)
	text := strings.TrimSpace(msgMeta.Body)
	files := legacyUIMessageFiles(msgMeta)
	toolCalls := legacyToolCalls(msgMeta.ToolCalls)
	toolOutputs := legacyToolOutputs(msgMeta.ToolCalls)

	bundle := make([]PromptMessage, 0, 2+len(toolOutputs))
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
			bundle = append(bundle, PromptMessage{
				Role:       PromptRoleToolResult,
				ToolCallID: toolOutput.callID,
				Blocks: []PromptBlock{{
					Type: PromptBlockText,
					Text: toolOutput.outputText,
				}},
			})
		}
		if injectImages && len(msgMeta.GeneratedFiles) > 0 {
			if generated := oc.generatedImagesHistoryMessage(ctx, msgMeta.GeneratedFiles); len(generated.Blocks) > 0 {
				bundle = append(bundle, generated)
			}
		}
	case "user":
		body := airuntime.SanitizeChatMessageForDisplay(text, true)
		if userMsg, ok := oc.canonicalUserHistoryMessage(ctx, body, files, injectImages); ok {
			return append(bundle, userMsg)
		}
		if body != "" {
			bundle = append(bundle, PromptMessage{
				Role: PromptRoleUser,
				Blocks: []PromptBlock{{
					Type: PromptBlockText,
					Text: body,
				}},
			})
		}
	}
	return bundle
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

func canonicalAssistantHistoryMessage(text string, toolCalls []canonicalToolCall) (PromptMessage, bool) {
	if text == "" && len(toolCalls) == 0 {
		return PromptMessage{}, false
	}

	assistant := PromptMessage{
		Role:   PromptRoleAssistant,
		Blocks: make([]PromptBlock, 0, 1+len(toolCalls)),
	}
	if text != "" {
		assistant.Blocks = append(assistant.Blocks, PromptBlock{
			Type: PromptBlockText,
			Text: text,
		})
	}
	for _, toolCall := range toolCalls {
		assistant.Blocks = append(assistant.Blocks, PromptBlock{
			Type:              PromptBlockToolCall,
			ToolCallID:        toolCall.callID,
			ToolName:          toolCall.toolName,
			ToolCallArguments: toolCall.arguments,
		})
	}
	return assistant, true
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
) (PromptMessage, bool) {
	parts := make([]PromptBlock, 0, len(files)+1)
	textWithURLs := body

	for _, file := range files {
		if file.URL == "" {
			continue
		}
		switch {
		case injectImages && isImageMimeType(file.MediaType):
			imgPart := oc.downloadHistoryImageBlock(ctx, file.URL, file.MediaType)
			if imgPart == nil {
				continue
			}
			if textWithURLs != "" {
				textWithURLs += "\n"
			}
			textWithURLs += fmt.Sprintf("[media_url: %s]", file.URL)
			parts = append(parts, *imgPart)
		case strings.HasPrefix(file.MediaType, "audio/"), strings.HasPrefix(file.MediaType, "video/"):
			if textWithURLs != "" {
				textWithURLs += "\n"
			}
			textWithURLs += fmt.Sprintf("[media_url: %s]", file.URL)
		default:
			filePart := oc.downloadHistoryFileBlock(ctx, file)
			if filePart != nil {
				parts = append(parts, *filePart)
			}
		}
	}

	if textWithURLs != "" {
		parts = append([]PromptBlock{{
			Type: PromptBlockText,
			Text: textWithURLs,
		}}, parts...)
	}
	if len(parts) == 0 {
		return PromptMessage{}, false
	}

	return PromptMessage{
		Role:   PromptRoleUser,
		Blocks: parts,
	}, true
}

func (oc *AIClient) generatedImagesHistoryMessage(ctx context.Context, files []GeneratedFileRef) PromptMessage {
	if len(files) == 0 {
		return PromptMessage{}
	}
	blocks := make([]PromptBlock, 0, 1+len(files))
	var sb strings.Builder
	sb.WriteString("[Previously generated image(s) for reference]")
	for _, f := range files {
		if !isImageMimeType(f.MimeType) || strings.TrimSpace(f.URL) == "" {
			continue
		}
		fmt.Fprintf(&sb, "\n[media_url: %s]", f.URL)
		if imgPart := oc.downloadHistoryImageBlock(ctx, f.URL, f.MimeType); imgPart != nil {
			blocks = append(blocks, *imgPart)
		}
	}
	if len(blocks) == 0 {
		return PromptMessage{}
	}
	blocks = append([]PromptBlock{{
		Type: PromptBlockText,
		Text: sb.String(),
	}}, blocks...)
	return PromptMessage{
		Role:   PromptRoleUser,
		Blocks: blocks,
	}
}

func (oc *AIClient) downloadHistoryFileBlock(ctx context.Context, file canonicalFilePart) *PromptBlock {
	b64Data, actualMimeType, err := oc.downloadMediaBase64(ctx, file.URL, nil, 50, file.MediaType)
	if err != nil {
		oc.log.Debug().Err(err).Str("url", file.URL).Msg("Failed to download history file, skipping")
		return nil
	}
	return &PromptBlock{
		Type:     PromptBlockFile,
		FileB64:  buildDataURL(actualMimeType, b64Data),
		Filename: file.Filename,
		MimeType: actualMimeType,
	}
}

func (oc *AIClient) downloadHistoryImageBlock(ctx context.Context, mediaURL, mimeType string) *PromptBlock {
	b64Data, actualMimeType, err := oc.downloadMediaBase64(ctx, mediaURL, nil, 25, mimeType)
	if err != nil {
		oc.log.Debug().Err(err).Str("url", mediaURL).Msg("Failed to download history image, skipping")
		return nil
	}
	return &PromptBlock{
		Type:     PromptBlockImage,
		ImageB64: b64Data,
		MimeType: actualMimeType,
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
