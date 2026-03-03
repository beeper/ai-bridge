package connector

import (
	"context"
	"strings"

	"github.com/beeper/ai-bridge/pkg/agents"
)

type heartbeatSessionResolution struct {
	StoreRef   sessionStoreRef
	SessionKey string
	Entry      *sessionEntry
}

func (oc *AIClient) resolveHeartbeatSession(agentID string, heartbeat *HeartbeatConfig) heartbeatSessionResolution {
	var cfg *Config
	if oc != nil && oc.connector != nil {
		cfg = &oc.connector.Config
	}
	resolvedAgent := normalizeAgentID(agentID)
	if resolvedAgent == "" {
		resolvedAgent = normalizeAgentID(agents.DefaultAgentID)
	}
	storeRef, mainSessionKey := resolveHeartbeatStoreParams(oc, agentID)
	scope := sessionScopePerSender
	if cfg != nil && cfg.Session != nil {
		scope = normalizeSessionScope(cfg.Session.Scope)
	}
	store, _ := oc.loadSessionStore(context.Background(), storeRef)

	// Helper to build a resolution, optionally attaching an entry if found.
	makeResolution := func(key string) heartbeatSessionResolution {
		if entry, ok := store.Sessions[key]; ok {
			entryCopy := entry
			return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: key, Entry: &entryCopy}
		}
		return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: key}
	}

	if scope == sessionScopeGlobal {
		return makeResolution(mainSessionKey)
	}

	trimmed := ""
	if heartbeat != nil && heartbeat.Session != nil {
		trimmed = strings.TrimSpace(*heartbeat.Session)
	}
	if trimmed == "" || strings.EqualFold(trimmed, "main") || strings.EqualFold(trimmed, "global") {
		return makeResolution(mainSessionKey)
	}

	if strings.HasPrefix(trimmed, "!") {
		return makeResolution(trimmed)
	}

	candidate := toAgentStoreSessionKey(resolvedAgent, trimmed, "")
	if cfg != nil && cfg.Session != nil {
		candidate = toAgentStoreSessionKey(resolvedAgent, trimmed, cfg.Session.MainKey)
	}
	canonical := canonicalizeMainSessionAlias(cfg, resolvedAgent, candidate)
	if canonical != sessionScopeGlobal && resolveAgentIdFromSessionKey(canonical) == resolvedAgent {
		return makeResolution(canonical)
	}

	return makeResolution(mainSessionKey)
}

func (oc *AIClient) resolveHeartbeatMainSessionRef(agentID string) (sessionStoreRef, string) {
	return resolveHeartbeatStoreParams(oc, agentID)
}

// resolveHeartbeatStoreParams computes the store ref and main session key
// without loading the session store (lightweight, no I/O).
func resolveHeartbeatStoreParams(oc *AIClient, agentID string) (sessionStoreRef, string) {
	var cfg *Config
	if oc != nil && oc.connector != nil {
		cfg = &oc.connector.Config
	}
	resolvedAgent := normalizeAgentID(agentID)
	if resolvedAgent == "" {
		resolvedAgent = normalizeAgentID(agents.DefaultAgentID)
	}
	scope := sessionScopePerSender
	if cfg != nil && cfg.Session != nil {
		scope = normalizeSessionScope(cfg.Session.Scope)
	}
	mainSessionKey := resolveAgentMainSessionKey(cfg, resolvedAgent)
	if scope == sessionScopeGlobal {
		mainSessionKey = sessionScopeGlobal
	}
	storeAgentID := resolvedAgent
	if scope == sessionScopeGlobal {
		storeAgentID = normalizeAgentID(agents.DefaultAgentID)
		if storeAgentID == "" {
			storeAgentID = resolvedAgent
		}
	}
	return sessionStoreRef{
		AgentID: storeAgentID,
		Path:    resolveSessionStorePath(cfg, storeAgentID),
	}, mainSessionKey
}
