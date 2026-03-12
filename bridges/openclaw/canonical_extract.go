package openclaw

import (
	"strings"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/pkg/shared/maputil"
	"github.com/beeper/agentremote/pkg/shared/stringutil"
)

func openClawCanonicalReasoningText(uiMessage map[string]any) string {
	parts := normalizeOpenClawUIParts(uiMessage["parts"])
	var sb strings.Builder
	for _, part := range parts {
		if maputil.StringArg(part, "type") != "reasoning" {
			continue
		}
		text := maputil.StringArg(part, "text")
		if text == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(text)
	}
	return sb.String()
}

func openClawCanonicalToolCalls(uiMessage map[string]any) []agentremote.ToolCallMetadata {
	parts := normalizeOpenClawUIParts(uiMessage["parts"])
	var calls []agentremote.ToolCallMetadata
	for _, raw := range parts {
		if maputil.StringArg(raw, "type") != "dynamic-tool" {
			continue
		}
		call := agentremote.ToolCallMetadata{
			CallID:   maputil.StringArg(raw, "toolCallId"),
			ToolName: maputil.StringArg(raw, "toolName"),
			ToolType: "openclaw",
			Status:   maputil.StringArg(raw, "state"),
		}
		if input, ok := raw["input"].(map[string]any); ok {
			call.Input = input
		}
		if output, ok := raw["output"].(map[string]any); ok {
			call.Output = output
		} else if text := maputil.StringArg(raw, "output"); text != "" {
			call.Output = map[string]any{"text": text}
		}
		switch call.Status {
		case "output-available":
			call.ResultStatus = "completed"
		case "output-denied":
			call.ResultStatus = "denied"
		case "output-error":
			call.ResultStatus = "error"
			call.ErrorMessage = maputil.StringArg(raw, "errorText")
		case "approval-requested":
			call.ResultStatus = "pending_approval"
		default:
			call.ResultStatus = call.Status
		}
		if call.CallID != "" {
			calls = append(calls, call)
		}
	}
	return calls
}

func openClawCanonicalGeneratedFiles(uiMessage map[string]any) []agentremote.GeneratedFileRef {
	parts := normalizeOpenClawUIParts(uiMessage["parts"])
	var refs []agentremote.GeneratedFileRef
	for _, part := range parts {
		if maputil.StringArg(part, "type") != "file" {
			continue
		}
		url := maputil.StringArg(part, "url")
		if url == "" {
			continue
		}
		refs = append(refs, agentremote.GeneratedFileRef{
			URL:      url,
			MimeType: stringutil.FirstNonEmpty(maputil.StringArg(part, "mediaType"), "application/octet-stream"),
		})
	}
	return refs
}
