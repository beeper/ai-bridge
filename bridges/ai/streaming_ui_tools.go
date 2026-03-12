package ai

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote"
)

func (oc *AIClient) emitUIToolApprovalRequest(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	approvalID string,
	toolCallID string,
	toolName string,
	presentation agentremote.ApprovalPromptPresentation,
	targetEventID id.EventID,
	ttlSeconds int,
) {
	approvalID = strings.TrimSpace(approvalID)
	toolCallID = strings.TrimSpace(toolCallID)
	toolName = strings.TrimSpace(toolName)
	if approvalID == "" || toolCallID == "" {
		return
	}
	if toolName == "" {
		toolName = "tool"
	}

	// Emit stream event for real-time UI
	oc.uiEmitter(state).EmitUIToolApprovalRequest(ctx, portal, approvalID, toolCallID)

	turnID := ""
	if state != nil {
		turnID = state.turnID
	}
	oc.approvalFlow.SendPrompt(ctx, portal, agentremote.SendPromptParams{
		ApprovalPromptMessageParams: agentremote.ApprovalPromptMessageParams{
			ApprovalID:     approvalID,
			ToolCallID:     toolCallID,
			ToolName:       toolName,
			TurnID:         turnID,
			Presentation:   presentation,
			ReplyToEventID: targetEventID,
			ExpiresAt:      agentremote.ComputeApprovalExpiry(ttlSeconds),
		},
		RoomID:    portal.MXID,
		OwnerMXID: oc.UserLogin.UserMXID,
	})
}
