package connector

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func (oc *AIClient) buildUIMessageMetadata(state *streamingState, meta *PortalMetadata, includeUsage bool) map[string]any {
	metadata := map[string]any{
		"model":    oc.effectiveModel(meta),
		"turn_id":  state.turnID,
		"agent_id": state.agentID,
	}
	if includeUsage {
		metadata["usage"] = map[string]any{
			"prompt_tokens":     state.promptTokens,
			"completion_tokens": state.completionTokens,
			"reasoning_tokens":  state.reasoningTokens,
			"total_tokens":      state.totalTokens,
		}
		metadata["timing"] = map[string]any{
			"started_at":     state.startedAtMs,
			"first_token_at": state.firstTokenAtMs,
			"completed_at":   state.completedAtMs,
		}
		metadata["finish_reason"] = mapFinishReason(state.finishReason)
	}
	return metadata
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "stop"
	case "end_turn", "end-turn":
		return "stop"
	case "length", "max_output_tokens":
		return "length"
	case "content_filter", "content-filter":
		return "content-filter"
	case "tool_calls", "tool-calls", "tool_use", "tool-use", "toolUse":
		return "tool-calls"
	case "error":
		return "error"
	default:
		return "other"
	}
}

func shouldContinueChatToolLoop(finishReason string, toolCallCount int) bool {
	if toolCallCount <= 0 {
		return false
	}
	// Some providers/adapters report inconsistent finish reasons (e.g. "stop") even when
	// tool calls are present in the stream. The presence of tool calls is the reliable
	// signal that we must continue after sending tool results.
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "error", "cancelled":
		return false
	default:
		return true
	}
}

func maybePrependTextSeparator(state *streamingState, rawDelta string) string {
	if state == nil || !state.needsTextSeparator {
		return rawDelta
	}
	// Keep waiting until we see a non-whitespace delta; some providers stream whitespace separately.
	if strings.TrimSpace(rawDelta) == "" {
		return rawDelta
	}
	// If we don't have any visible text yet, don't inject anything.
	if state.visibleAccumulated.Len() == 0 {
		state.needsTextSeparator = false
		return rawDelta
	}

	// Only insert when both sides are non-whitespace; avoids double-spacing if the model already
	// starts the new round with whitespace/newlines.
	vis := state.visibleAccumulated.String()
	last, _ := utf8.DecodeLastRuneInString(vis)
	first, _ := utf8.DecodeRuneInString(rawDelta)
	state.needsTextSeparator = false
	if unicode.IsSpace(last) || unicode.IsSpace(first) {
		return rawDelta
	}
	// Newline is rendered as whitespace in Markdown/HTML, preventing word run-ons.
	return "\n" + rawDelta
}
