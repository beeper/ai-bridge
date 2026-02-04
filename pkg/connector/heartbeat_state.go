package connector

import (
	"context"
	"strings"
	"time"
)

const heartbeatDedupeWindowMs = 24 * 60 * 60 * 1000

func (oc *AIClient) isDuplicateHeartbeat(sessionKey string, text string, nowMs int64) bool {
	if oc == nil {
		return false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return false
	}
	entry, ok := oc.getHeartbeatSessionEntry(context.Background(), sessionKey)
	if !ok {
		return false
	}
	if strings.TrimSpace(entry.LastHeartbeatText) != trimmed {
		return false
	}
	if entry.LastHeartbeatSentAt <= 0 {
		return false
	}
	if nowMs-entry.LastHeartbeatSentAt < heartbeatDedupeWindowMs {
		return true
	}
	return false
}

func (oc *AIClient) recordHeartbeatText(sessionKey string, text string, sentAt int64) {
	if oc == nil {
		return
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}
	if sentAt <= 0 {
		sentAt = time.Now().UnixMilli()
	}
	oc.updateHeartbeatSessionEntry(context.Background(), sessionKey, func(entry heartbeatSessionEntry) heartbeatSessionEntry {
		entry.LastHeartbeatText = trimmed
		entry.LastHeartbeatSentAt = sentAt
		entry.UpdatedAt = heartbeatUpdatedAtNow()
		return entry
	})
}
