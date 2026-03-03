package connector

import (
	"context"
	"strings"
	"time"
)

const heartbeatDedupeWindowMs = 24 * 60 * 60 * 1000

func (oc *AIClient) isDuplicateHeartbeat(ref sessionStoreRef, sessionKey string, text string, nowMs int64) bool {
	trimmed := strings.TrimSpace(text)
	sessionKey = strings.TrimSpace(sessionKey)
	if oc == nil || trimmed == "" || sessionKey == "" {
		return false
	}
	entry, ok := oc.getSessionEntry(context.Background(), ref, sessionKey)
	if !ok || entry.LastHeartbeatSentAt <= 0 {
		return false
	}
	return strings.TrimSpace(entry.LastHeartbeatText) == trimmed &&
		nowMs-entry.LastHeartbeatSentAt < heartbeatDedupeWindowMs
}

func (oc *AIClient) recordHeartbeatText(ref sessionStoreRef, sessionKey string, text string, sentAt int64) {
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
	oc.updateSessionEntry(context.Background(), ref, sessionKey, func(entry sessionEntry) sessionEntry {
		patch := sessionEntry{
			LastHeartbeatText:   trimmed,
			LastHeartbeatSentAt: sentAt,
		}
		return mergeSessionEntry(entry, patch)
	})
}

func (oc *AIClient) restoreHeartbeatUpdatedAt(ref sessionStoreRef, sessionKey string, updatedAt int64) {
	if oc == nil {
		return
	}
	if updatedAt <= 0 {
		return
	}
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}
	entry, ok := oc.getSessionEntry(context.Background(), ref, sessionKey)
	if !ok {
		return
	}
	if entry.UpdatedAt >= updatedAt {
		return
	}
	oc.updateSessionEntry(context.Background(), ref, sessionKey, func(entry sessionEntry) sessionEntry {
		if entry.UpdatedAt < updatedAt {
			entry.UpdatedAt = updatedAt
		}
		return entry
	})
}
