package connector

import (
	"context"
	"fmt"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func ensureConvertedMessageParts(converted *bridgev2.ConvertedMessage) {
	if converted == nil || len(converted.Parts) == 0 {
		return
	}
	parts := converted.Parts[:0]
	for _, part := range converted.Parts {
		if part == nil {
			continue
		}
		if part.Content == nil {
			part.Content = &event.MessageEventContent{}
		}
		parts = append(parts, part)
	}
	converted.Parts = parts
}

// validatePortal checks that a portal is usable for sending events.
func validatePortal(portal *bridgev2.Portal) error {
	if portal == nil || portal.MXID == "" {
		return fmt.Errorf("invalid portal")
	}
	return nil
}

// queueResultError converts a QueueRemoteEvent result into an error with the given operation label.
func queueResultError(success bool, err error, operation string) error {
	if success {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	return fmt.Errorf("%s failed", operation)
}

// sendViaPortal handles intent resolution, ghost room join, send, and DB persist via QueueRemoteEvent.
// Returns the Matrix event ID and the network message ID used.
// If msgID is empty, a new one is generated.
func (oc *AIClient) sendViaPortal(
	ctx context.Context,
	portal *bridgev2.Portal,
	converted *bridgev2.ConvertedMessage,
	msgID networkid.MessageID,
) (id.EventID, networkid.MessageID, error) {
	if err := validatePortal(portal); err != nil {
		return "", "", err
	}
	if msgID == "" {
		msgID = newMessageID()
	}
	ensureConvertedMessageParts(converted)
	sender := oc.senderForPortal(ctx, portal)
	result := oc.UserLogin.QueueRemoteEvent(&AIRemoteMessage{
		portal:    portal.PortalKey,
		id:        msgID,
		sender:    sender,
		timestamp: time.Now(),
		preBuilt:  converted,
	})
	if err := queueResultError(result.Success, result.Error, "send"); err != nil {
		return "", msgID, err
	}
	return result.EventID, msgID, nil
}

// sendEditViaPortal sends an edit for the message identified by targetMsgID.
func (oc *AIClient) sendEditViaPortal(
	ctx context.Context,
	portal *bridgev2.Portal,
	targetMsgID networkid.MessageID,
	converted *bridgev2.ConvertedEdit,
) error {
	if err := validatePortal(portal); err != nil {
		return err
	}
	sender := oc.senderForPortal(ctx, portal)
	result := oc.UserLogin.QueueRemoteEvent(&AIRemoteEdit{
		portal:        portal.PortalKey,
		sender:        sender,
		targetMessage: targetMsgID,
		timestamp:     time.Now(),
		preBuilt:      converted,
	})
	return queueResultError(result.Success, result.Error, "edit")
}

func (oc *AIClient) redactViaPortal(
	ctx context.Context,
	portal *bridgev2.Portal,
	targetMsgID networkid.MessageID,
) error {
	if err := validatePortal(portal); err != nil {
		return err
	}
	sender := oc.senderForPortal(ctx, portal)
	result := oc.UserLogin.QueueRemoteEvent(&AIRemoteMessageRemove{
		portal:        portal.PortalKey,
		sender:        sender,
		targetMessage: targetMsgID,
	})
	return queueResultError(result.Success, result.Error, "redact")
}

// redactEventViaPortal redacts a single Matrix event by its event ID through bridgev2's pipeline.
// Unlike redactViaPortal, this looks up the message by Matrix event ID rather than network message ID.
func (oc *AIClient) redactEventViaPortal(
	ctx context.Context,
	portal *bridgev2.Portal,
	eventID id.EventID,
) error {
	if portal == nil || portal.MXID == "" || eventID == "" {
		return fmt.Errorf("invalid portal or event ID")
	}
	part, err := oc.UserLogin.Bridge.DB.Message.GetPartByMXID(ctx, eventID)
	if err != nil {
		return fmt.Errorf("message lookup failed: %w", err)
	}
	if part == nil {
		return fmt.Errorf("message not found for event %s", eventID)
	}
	return oc.redactViaPortal(ctx, portal, part.ID)
}

// Use this when you need an intent for non-message operations (e.g. UploadMedia).
func (oc *AIClient) getIntentForPortal(
	ctx context.Context,
	portal *bridgev2.Portal,
	evtType bridgev2.RemoteEventType,
) (bridgev2.MatrixAPI, error) {
	sender := oc.senderForPortal(ctx, portal)
	intent, ok := portal.GetIntentFor(ctx, sender, oc.UserLogin, evtType)
	if !ok {
		return nil, fmt.Errorf("intent resolution failed")
	}
	return intent, nil
}

func (oc *AIClient) senderForPortal(ctx context.Context, portal *bridgev2.Portal) bridgev2.EventSender {
	meta := portalMeta(portal)
	agentID := resolveAgentID(meta)
	modelID := oc.effectiveModel(meta)
	if agentID == "" {
		if override, ok := modelOverrideFromContext(ctx); ok {
			modelID = override
		}
	}
	senderID := modelUserID(modelID)
	if agentID != "" {
		senderID = agentUserID(agentID)
	}
	return bridgev2.EventSender{Sender: senderID, SenderLogin: oc.UserLogin.ID}
}
