//lint:file-ignore U1000 Hard-cut compatibility: pending full dead-code deletion.
package connector

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultSessionStorePath = "sessions/sessions.json"

type sessionEntry struct {
	SessionID           string `json:"sessionId,omitempty"`
	UpdatedAt           int64  `json:"updatedAt,omitempty"`
	LastHeartbeatText   string `json:"lastHeartbeatText,omitempty"`
	LastHeartbeatSentAt int64  `json:"lastHeartbeatSentAt,omitempty"`
	LastChannel         string `json:"lastChannel,omitempty"`
	LastTo              string `json:"lastTo,omitempty"`
	LastAccountID       string `json:"lastAccountId,omitempty"`
	LastThreadID        string `json:"lastThreadId,omitempty"`
	QueueMode           string `json:"queueMode,omitempty"`
	QueueDebounceMs     *int   `json:"queueDebounceMs,omitempty"`
	QueueCap            *int   `json:"queueCap,omitempty"`
	QueueDrop           string `json:"queueDrop,omitempty"`
}

type sessionStore struct {
	Sessions map[string]sessionEntry `json:"sessions"`
}

type sessionStoreRef struct {
	AgentID string
	Path    string
}

var (
	sessionStoreLocks sync.Map
	sessionStores     sync.Map // key -> *sessionStore
)

func sessionStoreLockKey(ref sessionStoreRef) string {
	agent := strings.TrimSpace(ref.AgentID)
	path := strings.TrimSpace(ref.Path)
	if agent == "" {
		agent = "main"
	}
	if path == "" {
		path = defaultSessionStorePath
	}
	return agent + "|" + path
}

func sessionStoreLock(ref sessionStoreRef) *sync.Mutex {
	key := sessionStoreLockKey(ref)
	if val, ok := sessionStoreLocks.Load(key); ok {
		return val.(*sync.Mutex)
	}
	mu := &sync.Mutex{}
	actual, _ := sessionStoreLocks.LoadOrStore(key, mu)
	return actual.(*sync.Mutex)
}

func resolveSessionStorePath(cfg *Config, agentID string) string {
	raw := ""
	if cfg != nil && cfg.Session != nil {
		raw = cfg.Session.Store
	}
	normalizedAgent := normalizeAgentID(agentID)
	if normalizedAgent == "" {
		normalizedAgent = normalizeAgentID(defaultAgentID)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultSessionStorePath
	}
	expanded := strings.ReplaceAll(trimmed, "{agentId}", normalizedAgent)
	if strings.HasPrefix(expanded, "~") {
		expanded = strings.TrimPrefix(expanded, "~")
		expanded = strings.TrimPrefix(expanded, "/")
	}
	expanded = strings.TrimPrefix(strings.TrimSpace(expanded), "/")
	if expanded == "" {
		return defaultSessionStorePath
	}
	return expanded
}

func loadSessionStoreFromMemory(ref sessionStoreRef) sessionStore {
	key := sessionStoreLockKey(ref)
	if raw, ok := sessionStores.Load(key); ok {
		if st, ok := raw.(*sessionStore); ok && st != nil {
			copyStore := sessionStore{Sessions: map[string]sessionEntry{}}
			for k, v := range st.Sessions {
				copyStore.Sessions[k] = v
			}
			return copyStore
		}
	}
	return sessionStore{Sessions: map[string]sessionEntry{}}
}

func saveSessionStoreToMemory(ref sessionStoreRef, store sessionStore) {
	if store.Sessions == nil {
		store.Sessions = map[string]sessionEntry{}
	}
	key := sessionStoreLockKey(ref)
	copyStore := &sessionStore{Sessions: map[string]sessionEntry{}}
	for k, v := range store.Sessions {
		copyStore.Sessions[k] = v
	}
	sessionStores.Store(key, copyStore)
}

func (oc *AIClient) loadSessionStore(_ context.Context, ref sessionStoreRef) (sessionStore, error) {
	return loadSessionStoreFromMemory(ref), nil
}

func (oc *AIClient) saveSessionStore(_ context.Context, ref sessionStoreRef, store sessionStore) error {
	saveSessionStoreToMemory(ref, store)
	return nil
}

func (oc *AIClient) getSessionEntry(ctx context.Context, ref sessionStoreRef, sessionKey string) (sessionEntry, bool) {
	if oc == nil || strings.TrimSpace(sessionKey) == "" {
		return sessionEntry{}, false
	}
	store, err := oc.loadSessionStore(ctx, ref)
	if err != nil {
		oc.Log().Warn().Err(err).Str("session_key", sessionKey).Msg("session store: load failed in getSessionEntry")
		return sessionEntry{}, false
	}
	entry, ok := store.Sessions[sessionKey]
	return entry, ok
}

func (oc *AIClient) updateSessionEntry(ctx context.Context, ref sessionStoreRef, sessionKey string, updater func(entry sessionEntry) sessionEntry) {
	if oc == nil || strings.TrimSpace(sessionKey) == "" {
		return
	}
	lock := sessionStoreLock(ref)
	lock.Lock()
	defer lock.Unlock()
	store, err := oc.loadSessionStore(ctx, ref)
	if err != nil {
		oc.Log().Warn().Err(err).Str("session_key", sessionKey).Msg("session store: load failed in updateSessionEntry")
		return
	}
	entry := store.Sessions[sessionKey]
	entry = updater(entry)
	store.Sessions[sessionKey] = entry
	if err := oc.saveSessionStore(ctx, ref, store); err != nil {
		oc.Log().Warn().Err(err).Str("session_key", sessionKey).Msg("session store: save failed in updateSessionEntry")
	}
}

func mergeSessionEntry(existing sessionEntry, patch sessionEntry) sessionEntry {
	sessionID := patch.SessionID
	if sessionID == "" {
		sessionID = existing.SessionID
	}
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	updatedAt := time.Now().UnixMilli()
	if existing.UpdatedAt > updatedAt {
		updatedAt = existing.UpdatedAt
	}
	if patch.UpdatedAt > updatedAt {
		updatedAt = patch.UpdatedAt
	}
	next := existing
	if patch.LastHeartbeatText != "" {
		next.LastHeartbeatText = patch.LastHeartbeatText
	}
	if patch.LastHeartbeatSentAt != 0 {
		next.LastHeartbeatSentAt = patch.LastHeartbeatSentAt
	}
	if patch.LastChannel != "" {
		next.LastChannel = patch.LastChannel
	}
	if patch.LastTo != "" {
		next.LastTo = patch.LastTo
	}
	if patch.LastAccountID != "" {
		next.LastAccountID = patch.LastAccountID
	}
	if patch.LastThreadID != "" {
		next.LastThreadID = patch.LastThreadID
	}
	if patch.QueueMode != "" {
		next.QueueMode = patch.QueueMode
	}
	if patch.QueueDebounceMs != nil {
		next.QueueDebounceMs = patch.QueueDebounceMs
	}
	if patch.QueueCap != nil {
		next.QueueCap = patch.QueueCap
	}
	if patch.QueueDrop != "" {
		next.QueueDrop = patch.QueueDrop
	}
	next.SessionID = sessionID
	next.UpdatedAt = updatedAt
	return next
}

// resolveSessionStoreRef returns the session store ref used for this session key space.
func (oc *AIClient) resolveSessionStoreRef(agentID string) sessionStoreRef {
	cfg := (*Config)(nil)
	if oc != nil && oc.connector != nil {
		cfg = &oc.connector.Config
	}
	normalized := normalizeAgentID(agentID)
	if normalized == "" {
		normalized = normalizeAgentID(defaultAgentID)
	}
	path := resolveSessionStorePath(cfg, normalized)
	return sessionStoreRef{AgentID: normalized, Path: path}
}
