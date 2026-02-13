package connector

import (
	"bytes"
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"
)

type activeToolCall struct {
	callID      string
	toolName    string
	toolType    ToolType
	startedAtMs int64
	itemID      string
	eventID     id.EventID
	input       bytes.Buffer
	result      string
}

func toolDisplayTitle(toolName string) string { return toolName }

func (oc *AIClient) sendToolCallApprovalEvent(context.Context, *bridgev2.Portal, *streamingState, string, string, string, int64) {}

func (oc *AIClient) sendToolCallEvent(context.Context, *bridgev2.Portal, *streamingState, *activeToolCall) id.EventID {
	return ""
}

func (oc *AIClient) sendToolResultEvent(context.Context, *bridgev2.Portal, *streamingState, *activeToolCall, string, ResultStatus) id.EventID {
	return ""
}
