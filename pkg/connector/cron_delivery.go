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
	if strings.EqualFold(channel, "last") {
		if to := strings.TrimSpace(payload.To); to != "" {
			if strings.HasPrefix(to, "!") {
				if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(to)); err == nil && portal != nil {
					return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
				}
				return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
			}
			return cronDeliveryTarget{Channel: "matrix", Reason: "invalid-target"}
		}
		if portal := oc.lastActivePortal(agentID); portal != nil {
			return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
		}
		return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
	}

	if strings.EqualFold(channel, "matrix") {
		if to := strings.TrimSpace(payload.To); to != "" {
			if strings.HasPrefix(to, "!") {
				if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(to)); err == nil && portal != nil {
					return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
				}
				return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
			}
			return cronDeliveryTarget{Channel: "matrix", Reason: "invalid-target"}
		}
		if portal := oc.lastActivePortal(agentID); portal != nil {
			return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
		}
		return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
	}

	if strings.HasPrefix(channel, "!") {
		if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(channel)); err == nil && portal != nil {
			return cronDeliveryTarget{Portal: portal, RoomID: portal.MXID, Channel: "matrix"}
		}
		return cronDeliveryTarget{Channel: "matrix", Reason: "no-target"}
	}

	return cronDeliveryTarget{Channel: strings.ToLower(channel), Reason: "unsupported-channel"}
}
