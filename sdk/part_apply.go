package sdk

import "strings"

// PartApplyOptions controls provider-specific edge cases when applying
// streamed UI/tool parts to a turn.
type PartApplyOptions struct {
	ResetMetadataOnStartMarkers     bool
	ResetMetadataOnEmptyMessageMeta bool
	ResetMetadataOnEmptyTextDelta   bool
	ResetMetadataOnAbort            bool
	ResetMetadataOnDataParts        bool
	HandleTerminalEvents            bool
	DefaultFinishReason             string
}

// ApplyStreamPart maps a canonical stream part onto a turn. It returns true when
// the part type is recognized and applied.
func ApplyStreamPart(turn *Turn, part map[string]any, opts PartApplyOptions) bool {
	if turn == nil || len(part) == 0 {
		return false
	}
	partType := strings.TrimSpace(partString(part, "type"))
	if partType == "" {
		return false
	}
	writer := turn.Writer()
	tools := turn.Tools()
	switch partType {
	case "start", "message-metadata":
		metadata, _ := part["messageMetadata"].(map[string]any)
		if len(metadata) > 0 {
			turn.SetMetadata(metadata)
		} else if opts.ResetMetadataOnEmptyMessageMeta {
			turn.SetMetadata(nil)
		}
	case "start-step":
		turn.StepStart()
	case "finish-step":
		turn.StepFinish()
	case "text-start", "reasoning-start":
		if opts.ResetMetadataOnStartMarkers {
			turn.SetMetadata(nil)
		}
	case "text-delta":
		if delta := partString(part, "delta"); delta != "" {
			turn.WriteText(delta)
		} else if opts.ResetMetadataOnEmptyTextDelta {
			turn.SetMetadata(nil)
		}
	case "text-end":
		turn.FinishText()
	case "reasoning-delta":
		if delta := partString(part, "delta"); delta != "" {
			turn.WriteReasoning(delta)
		} else if opts.ResetMetadataOnEmptyTextDelta {
			turn.SetMetadata(nil)
		}
	case "reasoning-end":
		turn.FinishReasoning()
	case "tool-input-start":
		tools.EnsureInputStart(turn.Context(), partString(part, "toolCallId"), nil, ToolInputOptions{
			ToolName:         partString(part, "toolName"),
			ProviderExecuted: partBool(part, "providerExecuted"),
		})
	case "tool-input-delta":
		tools.InputDelta(turn.Context(), partString(part, "toolCallId"), "", partString(part, "inputTextDelta"), partBool(part, "providerExecuted"))
	case "tool-input-available":
		tools.Input(turn.Context(), partString(part, "toolCallId"), partString(part, "toolName"), part["input"], partBool(part, "providerExecuted"))
	case "tool-output-available":
		tools.Output(turn.Context(), partString(part, "toolCallId"), part["output"], ToolOutputOptions{
			ProviderExecuted: partBool(part, "providerExecuted"),
		})
	case "tool-output-error":
		tools.OutputError(turn.Context(), partString(part, "toolCallId"), partString(part, "errorText"), partBool(part, "providerExecuted"))
	case "tool-output-denied":
		tools.Denied(turn.Context(), partString(part, "toolCallId"))
	case "tool-approval-request":
		turn.Approvals().EmitRequest(turn.Context(), partString(part, "approvalId"), partString(part, "toolCallId"))
	case "tool-approval-response":
		turn.Approvals().Respond(turn.Context(), partString(part, "approvalId"), partString(part, "toolCallId"), partBool(part, "approved"), partString(part, "reason"))
	case "file":
		turn.AddFile(partString(part, "url"), partString(part, "mediaType"))
	case "source-document":
		turn.AddSourceDocument(partString(part, "sourceId"), partString(part, "title"), partString(part, "mediaType"), partString(part, "filename"))
	case "source-url":
		turn.AddSourceURL(partString(part, "url"), partString(part, "title"))
	case "error":
		turn.Error(partString(part, "errorText"))
	case "finish":
		if !opts.HandleTerminalEvents {
			return false
		}
		finishReason := partString(part, "finishReason")
		if finishReason == "" {
			finishReason = strings.TrimSpace(opts.DefaultFinishReason)
		}
		if finishReason == "" {
			finishReason = "stop"
		}
		turn.End(finishReason)
	case "abort":
		if !opts.HandleTerminalEvents {
			return false
		}
		if opts.ResetMetadataOnAbort {
			turn.SetMetadata(nil)
		}
		turn.Abort(partString(part, "reason"))
	default:
		if strings.HasPrefix(partType, "data-") {
			if opts.ResetMetadataOnDataParts {
				turn.SetMetadata(nil)
			}
			writer.RawPart(turn.Context(), part)
			return true
		}
		return false
	}
	return true
}

func partString(part map[string]any, key string) string {
	return strings.TrimSpace(stringValue(part[key]))
}

func partBool(part map[string]any, key string) bool {
	value, _ := part[key].(bool)
	return value
}
