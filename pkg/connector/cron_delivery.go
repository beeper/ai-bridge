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

func (oc *AIClient) resolveCronDeliveryTarget(agentID string, delivery *cron.CronDelivery) cronDeliveryTarget {
	if delivery == nil {
		return cronDeliveryTarget{Reason: "no-delivery"}
	}

	channel := strings.TrimSpace(delivery.Channel)
	if channel == "" {
		channel = "last"
	}
	lowered := strings.ToLower(channel)
	if lowered != "last" && lowered != "matrix" {
		return cronDeliveryTarget{Channel: lowered, Reason: "unsupported-channel"}
	}

	target := strings.TrimSpace(delivery.To)
	if target == "" && lowered == "last" {
		storeRef, mainKey := oc.resolveHeartbeatMainSessionRef(agentID)
		if entry, ok := oc.getSessionEntry(context.Background(), storeRef, mainKey); ok {
			lastChannel := strings.TrimSpace(entry.LastChannel)
			if lastChannel == "" || strings.EqualFold(lastChannel, "matrix") {
				target = strings.TrimSpace(entry.LastTo)
			}
		}
		if target == "" {
			if portal := oc.lastActivePortal(agentID); portal != nil && portal.MXID != "" {
				target = portal.MXID.String()
			}
		}
		if target == "" {
			if portal := oc.defaultChatPortal(); portal != nil && portal.MXID != "" {
				target = portal.MXID.String()
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
		if !oc.IsLoggedIn() {
			return cronDeliveryTarget{Channel: "matrix", Reason: "channel-not-ready"}
		}
		return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
	}
	return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
}
