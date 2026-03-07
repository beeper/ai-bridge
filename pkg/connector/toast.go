package connector

import (
	"context"
	"errors"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/bridgeadapter"
)

type aiToastType string

const (
	aiToastTypeError aiToastType = "error"
)

func (oc *AIClient) sendApprovalRequestFallbackEvent(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	approvalID string,
	toolCallID string,
	toolName string,
	replyToEventID id.EventID,
) {
	if oc == nil || portal == nil || portal.MXID == "" {
		return
	}
	approvalID = strings.TrimSpace(approvalID)
	toolCallID = strings.TrimSpace(toolCallID)
	toolName = strings.TrimSpace(toolName)
	if approvalID == "" || toolCallID == "" {
		return
	}
	if toolName == "" {
		toolName = "tool"
	}
	uiMessage := map[string]any{
		"id":   approvalID,
		"role": "assistant",
		"metadata": map[string]any{
			"turn_id":    state.turnID,
			"approvalId": approvalID,
		},
		"parts": []map[string]any{{
			"type":       "dynamic-tool",
			"toolName":   toolName,
			"toolCallId": toolCallID,
			"state":      "approval-requested",
			"approval": map[string]any{
				"id": approvalID,
			},
		}},
	}
	raw := map[string]any{
		"msgtype":    event.MsgNotice,
		"body":       "Tool approval required",
		BeeperAIKey:  uiMessage,
		"m.mentions": map[string]any{},
	}
	if replyToEventID != "" {
		raw["m.relates_to"] = map[string]any{
			"m.in_reply_to": map[string]any{
				"event_id": replyToEventID.String(),
			},
		}
	}
	converted := &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{{
			ID:      networkid.PartID("0"),
			Type:    event.EventMessage,
			Content: &event.MessageEventContent{MsgType: event.MsgNotice, Body: "Tool approval required"},
			Extra:   raw,
			DBMetadata: &MessageMetadata{
				Role:               "assistant",
				ExcludeFromHistory: true,
				CanonicalSchema:    "ai-sdk-ui-message-v1",
				CanonicalUIMessage: uiMessage,
			},
		}},
	}
	if _, _, err := oc.sendViaPortal(ctx, portal, converted, ""); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Str("approval_id", approvalID).Msg("Failed to send approval request fallback event")
	}
}

// sendApprovalRejectionEvent sends a combined toast + com.beeper.ai snapshot
// marking an approval as output-denied. This is used when resolveToolApproval
// fails (expired/unknown/already-handled) so the desktop can close the modal
// instead of retrying in a loop.
func (oc *AIClient) sendApprovalRejectionEvent(ctx context.Context, portal *bridgev2.Portal, approvalID string, err error, replyToEventID id.EventID) {
	if oc == nil || portal == nil || portal.MXID == "" || approvalID == "" {
		return
	}

	errorText := "Expired"
	switch {
	case errors.Is(err, bridgeadapter.ErrApprovalAlreadyHandled):
		errorText = "Already handled"
	case errors.Is(err, bridgeadapter.ErrApprovalOnlyOwner):
		errorText = "Denied"
	case errors.Is(err, bridgeadapter.ErrApprovalWrongRoom):
		errorText = "Denied"
	}

	toastText := bridgeadapter.ApprovalErrorToastText(err)
	raw := map[string]any{
		"msgtype": event.MsgNotice,
		"body":    toastText,
		"com.beeper.ai.toast": map[string]any{
			"text": toastText,
			"type": string(aiToastTypeError),
		},
		BeeperAIKey: map[string]any{
			"id":   approvalID,
			"role": "assistant",
			"metadata": map[string]any{
				"approvalId": approvalID,
			},
			"parts": []map[string]any{{
				"type":       "dynamic-tool",
				"toolName":   "tool",
				"toolCallId": approvalID,
				"state":      "output-denied",
				"approval": map[string]any{
					"id":       approvalID,
					"approved": false,
					"reason":   errorText,
				},
				"errorText": errorText,
			}},
		},
		"m.mentions": map[string]any{},
	}
	if replyToEventID != "" {
		raw["m.relates_to"] = map[string]any{
			"m.in_reply_to": map[string]any{
				"event_id": replyToEventID.String(),
			},
		}
	}
	converted := &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{{
			ID:    networkid.PartID("0"),
			Type:  event.EventMessage,
			Extra: raw,
		}},
	}
	if _, _, sendErr := oc.sendViaPortal(ctx, portal, converted, ""); sendErr != nil {
		oc.loggerForContext(ctx).Warn().Err(sendErr).Msg("Failed to send approval rejection event")
	}
}
