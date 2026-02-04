package connector

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/cron"
)

type cronDeliveryTarget struct {
	Portal  *bridgev2.Portal
	RoomID  id.RoomID
	Channel string
	Reason  string
}

func (oc *AIClient) resolveCronDeliveryTarget(agentID string, payload cron.CronPayload) cronDeliveryTarget {
	channel := strings.TrimSpace(payload.Channel)
	if channel == "" {
		channel = "last"
	}
	trimmedChannel := strings.TrimSpace(channel)
	if strings.HasPrefix(trimmedChannel, "!") {
		if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(trimmedChannel)); err == nil && portal != nil {
			return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
		}
		return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
	}

	lowered := strings.ToLower(trimmedChannel)
	if lowered != "last" && lowered != "matrix" {
		return cronDeliveryTarget{Channel: lowered, Reason: "unsupported-channel"}
	}

	target := strings.TrimSpace(payload.To)
	if target == "" {
		storeRef, mainKey := oc.resolveHeartbeatMainSessionRef(agentID)
		if entry, ok := oc.getSessionEntry(context.Background(), storeRef, mainKey); ok {
			lastChannel := strings.TrimSpace(entry.LastChannel)
			if lastChannel == "" || strings.EqualFold(lastChannel, "matrix") {
				target = strings.TrimSpace(entry.LastTo)
			}
		}
	}
	if target == "" {
		return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
	}
	if !strings.HasPrefix(target, "!") {
		return cronDeliveryTarget{Channel: "matrix", Reason: "invalid-target"}
	}
	if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(target)); err == nil && portal != nil {
		return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
	}
	return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
}
