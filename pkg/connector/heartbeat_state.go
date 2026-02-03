package connector

import (
	"context"
	"strings"
	"time"
)

const heartbeatDedupeWindowMs = 24 * 60 * 60 * 1000

func (oc *AIClient) isDuplicateHeartbeat(agentID, text string) bool {
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
	state, ok := meta.HeartbeatState[normalizeAgentID(agentID)]
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

func (oc *AIClient) recordHeartbeatText(agentID, text string) {
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
	key := normalizeAgentID(agentID)
	meta.HeartbeatState[key] = HeartbeatState{
		LastHeartbeatText:   trimmed,
		LastHeartbeatSentAt: time.Now().UnixMilli(),
	}
	_ = oc.UserLogin.Save(oc.backgroundContext(context.Background()))
}
