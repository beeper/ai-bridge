package connector

import (
	"context"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

const heartbeatDedupeWindowMs = 24 * 60 * 60 * 1000

func (oc *AIClient) isDuplicateHeartbeat(agentID string, roomID id.RoomID, text string) bool {
	if oc == nil || oc.UserLogin == nil {
		return false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	meta := loginMetadata(oc.UserLogin)
	if meta == nil || meta.HeartbeatState == nil {
		return false
	}
	state, ok := meta.HeartbeatState[heartbeatDedupeKey(agentID, roomID)]
	if !ok {
		return false
	}
	if strings.TrimSpace(state.LastHeartbeatText) == trimmed {
		if time.Now().UnixMilli()-state.LastHeartbeatSentAt < heartbeatDedupeWindowMs {
			return true
		}
	}
	return false
}

func (oc *AIClient) recordHeartbeatText(agentID string, roomID id.RoomID, text string) {
	if oc == nil || oc.UserLogin == nil {
		return
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	meta := loginMetadata(oc.UserLogin)
	if meta == nil {
		return
	}
	if meta.HeartbeatState == nil {
		meta.HeartbeatState = make(map[string]HeartbeatState)
	}
	key := heartbeatDedupeKey(agentID, roomID)
	meta.HeartbeatState[key] = HeartbeatState{
		LastHeartbeatText:   trimmed,
		LastHeartbeatSentAt: time.Now().UnixMilli(),
	}
	_ = oc.UserLogin.Save(oc.backgroundContext(context.Background()))
}

func heartbeatDedupeKey(agentID string, roomID id.RoomID) string {
	key := normalizeAgentID(agentID)
	if roomID == "" {
		return key
	}
	return key + ":" + roomID.String()
}
