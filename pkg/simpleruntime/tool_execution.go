//lint:file-ignore U1000 Tool execution event helpers are staged for future use.
package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// activeToolCall tracks a tool call that's in progress
type activeToolCall struct {
	callID      string
	toolName    string
	toolType    ToolType
	input       strings.Builder
	startedAtMs int64
	eventID     id.EventID // Event ID of the tool call timeline event
	result      string     // Result from tool execution (for continuation)
	itemID      string     // Item ID from the stream event (used as call_id for continuation)
}

func normalizeToolArgsJSON(argsJSON string) string {
	trimmed := strings.TrimSpace(argsJSON)
	if trimmed == "" || trimmed == "null" {
		return "{}"
	}
	return trimmed
}

func parseToolInputPayload(argsJSON string) map[string]any {
	trimmed := strings.TrimSpace(argsJSON)
	if trimmed == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return map[string]any{"_raw": trimmed}
	}
	if m, ok := parsed.(map[string]any); ok {
		return m
	}
	return map[string]any{"value": parsed}
}

func parseToolOutputPayload(result string) map[string]any {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return map[string]any{"result": result}
	}
	if m, ok := parsed.(map[string]any); ok {
		return m
	}
	return map[string]any{"result": parsed}
}

// emitToolProgress sends a tool progress update event
func (oc *AIClient) emitToolProgress(ctx context.Context, portal *bridgev2.Portal, state *streamingState, tool *activeToolCall, status ToolStatus, message string, percent int) {
	if state == nil || tool == nil {
		return
	}
	oc.emitStreamEvent(ctx, portal, state, map[string]any{
		"type": "data-tool-progress",
		"data": map[string]any{
			"call_id":   tool.callID,
			"tool_name": tool.toolName,
			"status":    string(status),
			"progress": map[string]any{
				"message": message,
				"percent": percent,
			},
		},
		"transient": true,
	})
}

func toolDisplayTitle(toolName string) string {
	toolName = normalizeToolAlias(toolName)
	return toolName
}

func summarizeMessageAction(obj map[string]any) string {
	action, _ := obj["action"].(string)
	switch action {
	case "react":
		emoji, _ := obj["emoji"].(string)
		status, _ := obj["status"].(string)
		if status == "removed" {
			if emoji != "" {
				return fmt.Sprintf("Removed reaction %s", emoji)
			}
			return "Removed reaction"
		}
		if emoji != "" {
			return fmt.Sprintf("Reacted with %s", emoji)
		}
		return "Reaction sent"
	case "send":
		return "Message sent"
	case "edit":
		return "Message edited"
	case "delete":
		return "Message deleted"
	case "reply":
		return "Reply sent"
	case "thread-reply":
		return "Thread reply sent"
	case "read":
		return "Read receipt sent"
	case "pin":
		return "Message pinned"
	case "unpin":
		return "Message unpinned"
	case "list-pins":
		return "Pins retrieved"
	case "reactions":
		return "Reactions retrieved"
	case "search":
		return "Search completed"
	case "member-info":
		return "Member info retrieved"
	case "channel-info":
		return "Channel info retrieved"
	case "channel-edit":
		return "Channel updated"
	default:
		return ""
	}
}

// sendToolCallEvent sends a tool call as a timeline event
func (oc *AIClient) sendToolCallEvent(ctx context.Context, portal *bridgev2.Portal, state *streamingState, tool *activeToolCall) id.EventID {
	if portal == nil || portal.MXID == "" {
		return ""
	}
	if state != nil && state.suppressSend {
		return ""
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	// Build display info
	displayTitle := toolDisplayTitle(tool.toolName)

	toolCallData := map[string]any{
		"call_id":   tool.callID,
		"turn_id":   state.turnID,
		"tool_name": tool.toolName,
		"tool_type": string(tool.toolType),
		"status":    string(ToolStatusRunning),
		"display": map[string]any{
			"title":     displayTitle,
			"collapsed": false,
		},
		"timing": map[string]any{
			"started_at": tool.startedAtMs,
		},
	}
	if input := parseToolInputPayload(tool.input.String()); len(input) > 0 {
		toolCallData["input"] = input
	}

	if state.agentID != "" {
		toolCallData["agent_id"] = state.agentID
	}

	eventRaw := map[string]any{
		"body":              fmt.Sprintf("Calling %s...", displayTitle),
		"msgtype":           event.MsgNotice,
		BeeperAIToolCallKey: toolCallData,
	}
	if state.initialEventID != "" {
		eventRaw["m.relates_to"] = map[string]any{
			"rel_type": RelReference,
			"event_id": state.initialEventID.String(),
		}
	}

	eventContent := &event.Content{Raw: eventRaw}

	resp, err := intent.SendMessage(ctx, portal.MXID, ToolCallEventType, eventContent, nil)
	if err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Str("tool", tool.toolName).Msg("Failed to send tool call event")
		return ""
	}

	oc.loggerForContext(ctx).Debug().
		Stringer("event_id", resp.EventID).
		Str("call_id", tool.callID).
		Str("tool", tool.toolName).
		Msg("Sent tool call timeline event")

	// Expose the Matrix event ID to the streaming UI so Desktop can react to the tool call event.
	if state != nil && tool != nil && strings.TrimSpace(tool.callID) != "" && resp.EventID != "" {
		oc.emitStreamEvent(ctx, portal, state, map[string]any{
			"type": "data-tool-call-event",
			"id":   fmt.Sprintf("tool-call-event:%s", tool.callID),
			"data": map[string]any{
				"toolCallId":  tool.callID,
				"callEventId": resp.EventID.String(),
			},
		})
	}

	return resp.EventID
}

// sendToolResultEvent sends a tool result as a timeline event
func (oc *AIClient) sendToolResultEvent(ctx context.Context, portal *bridgev2.Portal, state *streamingState, tool *activeToolCall, result string, resultStatus ResultStatus) id.EventID {
	if portal == nil || portal.MXID == "" {
		return ""
	}
	if state != nil && state.suppressSend {
		return ""
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	// Truncate result for body if too long
	bodyText := result
	var parsedResult any
	if err := json.Unmarshal([]byte(result), &parsedResult); err == nil {
		if obj, ok := parsedResult.(map[string]any); ok {
			if msg, ok := obj["message"].(string); ok && msg != "" {
				bodyText = msg
			} else if tool.toolName == ToolNameMessage {
				bodyText = summarizeMessageAction(obj)
			}
		}
	}
	if len(bodyText) > 200 {
		bodyText = bodyText[:200] + "..."
	}
	if bodyText == "" {
		bodyText = fmt.Sprintf("%s completed", toolDisplayTitle(tool.toolName))
	}

	toolResultData := map[string]any{
		"call_id":   tool.callID,
		"turn_id":   state.turnID,
		"tool_name": tool.toolName,
		"status":    string(resultStatus),
		"display": map[string]any{
			"expandable":       len(result) > 200,
			"default_expanded": len(result) <= 500,
		},
	}

	if state.agentID != "" {
		toolResultData["agent_id"] = state.agentID
	}

	if output := parseToolOutputPayload(result); len(output) > 0 {
		toolResultData["output"] = output
	}

	eventRaw := map[string]any{
		"body":                bodyText,
		"msgtype":             event.MsgNotice,
		BeeperAIToolResultKey: toolResultData,
	}
	if tool.eventID != "" {
		eventRaw["m.relates_to"] = map[string]any{
			"rel_type": RelReference,
			"event_id": tool.eventID.String(),
		}
	}

	eventContent := &event.Content{Raw: eventRaw}

	resp, err := intent.SendMessage(ctx, portal.MXID, ToolResultEventType, eventContent, nil)
	if err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Str("tool", tool.toolName).Msg("Failed to send tool result event")
		return ""
	}

	oc.loggerForContext(ctx).Debug().
		Stringer("event_id", resp.EventID).
		Str("call_id", tool.callID).
		Str("tool", tool.toolName).
		Str("status", string(resultStatus)).
		Msg("Sent tool result timeline event")

	return resp.EventID
}

// executeBuiltinTool finds and executes a builtin tool by name.
func (oc *AIClient) executeBuiltinTool(ctx context.Context, portal *bridgev2.Portal, toolName string, argsJSON string) (string, error) {
	argsJSON = normalizeToolArgsJSON(argsJSON)
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}

	toolName = normalizeToolAlias(toolName)
	if oc != nil && oc.isSimpleProfile() && toolName != ToolNameWebSearch {
		return "", fmt.Errorf("tool %s is not available in simple bridge", toolName)
	}

	// Standard builtin tools
	for _, tool := range BuiltinTools() {
		if tool.Name == toolName {
			return tool.Execute(ctx, args)
		}
	}
	return "", fmt.Errorf("unknown tool: %s", toolName)
}
