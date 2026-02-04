package connector

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"
)

type heartbeatDeliveryTarget struct {
	Portal  *bridgev2.Portal
	RoomID  id.RoomID
	Channel string
	Reason  string
}

func (oc *AIClient) resolveHeartbeatDeliveryTarget(agentID string, heartbeat *HeartbeatConfig, entry *sessionEntry) heartbeatDeliveryTarget {
	if oc == nil || oc.UserLogin == nil {
		return heartbeatDeliveryTarget{Reason: "no-target"}
	}
	if heartbeat != nil && heartbeat.Target != nil {
		if strings.EqualFold(strings.TrimSpace(*heartbeat.Target), "none") {
			return heartbeatDeliveryTarget{Reason: "target-none"}
		}
	}

	if heartbeat != nil && heartbeat.To != nil && strings.TrimSpace(*heartbeat.To) != "" {
		return oc.resolveHeartbeatDeliveryRoom(strings.TrimSpace(*heartbeat.To))
	}

	if heartbeat != nil && heartbeat.Target != nil {
		trimmed := strings.TrimSpace(*heartbeat.Target)
		if trimmed != "" && !strings.EqualFold(trimmed, "last") {
			return oc.resolveHeartbeatDeliveryRoom(trimmed)
		}
	}

	if entry != nil {
		lastChannel := strings.TrimSpace(entry.LastChannel)
		lastTo := strings.TrimSpace(entry.LastTo)
		if lastTo != "" && (lastChannel == "" || strings.EqualFold(lastChannel, "matrix")) {
			target := oc.resolveHeartbeatDeliveryRoom(lastTo)
			if target.Portal != nil && target.RoomID != "" {
				return target
			}
		}
	}

	return heartbeatDeliveryTarget{Reason: "no-target"}
}

func (oc *AIClient) resolveHeartbeatDeliveryRoom(raw string) heartbeatDeliveryTarget {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return heartbeatDeliveryTarget{Reason: "no-target"}
	}
	if !strings.HasPrefix(trimmed, "!") {
		return heartbeatDeliveryTarget{Reason: "no-target"}
	}
	portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(trimmed))
	if err != nil || portal == nil || portal.MXID == "" {
		return heartbeatDeliveryTarget{Reason: "no-target"}
	}
	return heartbeatDeliveryTarget{
		Portal:  portal,
		RoomID:  portal.MXID,
		Channel: "matrix",
	}
}
