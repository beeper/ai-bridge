package opencode

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/agentremote"
)

// sendViaPortal sends a pre-built message through bridgev2's QueueRemoteEvent pipeline.
func (oc *OpenCodeClient) sendViaPortal(
	_ context.Context,
	portal *bridgev2.Portal,
	instanceID string,
	converted *bridgev2.ConvertedMessage,
) error {
	_, _, err := oc.ClientBase.SendViaPortal(portal, oc.SenderForOpenCode(instanceID, false), converted)
	return err
}

// sendSystemNoticeViaPortal is a convenience wrapper for sending MsgNotice via the pipeline.
func (oc *OpenCodeClient) sendSystemNoticeViaPortal(ctx context.Context, portal *bridgev2.Portal, msg string) {
	pmeta := oc.PortalMeta(portal)
	instanceID := ""
	if pmeta != nil {
		instanceID = pmeta.InstanceID
	}
	if err := oc.sendViaPortal(ctx, portal, instanceID, agentremote.BuildSystemNotice(msg)); err != nil {
		oc.Log().Warn().Err(err).Msg("Failed to send system notice")
	}
}
