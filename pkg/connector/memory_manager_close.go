package connector

import (
	"context"
	"strings"
)

// Close stops background timers/goroutines and releases any dedicated DB connections.
// It is safe to call multiple times.
func (m *MemorySearchManager) Close() {
	if m == nil {
		return
	}

	// Stop interval ticker goroutine (if started).
	m.mu.Lock()
	stopCh := m.intervalStop
	m.mu.Unlock()
	if stopCh != nil {
		m.intervalStopOnce.Do(func() {
			close(stopCh)
		})
	}

	// Stop debounced timers and release vector connection.
	m.mu.Lock()
	if m.watchTimer != nil {
		m.watchTimer.Stop()
		m.watchTimer = nil
	}
	if m.sessionWatchTimer != nil {
		m.sessionWatchTimer.Stop()
		m.sessionWatchTimer = nil
	}
	conn := m.vectorConn
	m.vectorConn = nil
	m.vectorReady = false
	m.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
}

func purgeMemoryManagersForLogin(ctx context.Context, bridgeID, loginID string, chunkIDsByAgent map[string][]string) {
	if strings.TrimSpace(bridgeID) == "" || strings.TrimSpace(loginID) == "" {
		return
	}
	prefix := bridgeID + ":" + loginID + ":"

	memoryManagerCache.mu.Lock()
	managers := make([]*MemorySearchManager, 0, len(memoryManagerCache.managers))
	for key, mgr := range memoryManagerCache.managers {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if mgr != nil {
			managers = append(managers, mgr)
		}
		delete(memoryManagerCache.managers, key)
	}
	memoryManagerCache.mu.Unlock()

	// Best-effort: delete vector table rows using any existing vector-enabled managers.
	if ctx == nil {
		ctx = context.Background()
	}
	for _, mgr := range managers {
		if ids := chunkIDsByAgent[mgr.agentID]; len(ids) > 0 {
			mgr.deleteVectorIDs(ctx, ids)
		}
		mgr.Close()
	}
}
