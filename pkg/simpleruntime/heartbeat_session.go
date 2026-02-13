//go:build agentic

package connector

import (
	"context"
	"strings"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/agents"
)

type heartbeatSessionResolution struct {
	StoreRef   sessionStoreRef
	SessionKey string
	Entry      *sessionEntry
}

func (oc *AIClient) resolveHeartbeatSession(agentID string, heartbeat *HeartbeatConfig) heartbeatSessionResolution {
	cfg := (*Config)(nil)
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
	storeRef := sessionStoreRef{
		AgentID: storeAgentID,
		Path:    resolveSessionStorePath(cfg, storeAgentID),
	}
	store, _ := oc.loadSessionStore(context.Background(), storeRef)
	mainEntry, hasMain := store.Sessions[mainSessionKey]
	if scope == sessionScopeGlobal {
		if hasMain {
			entry := mainEntry
			return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: mainSessionKey, Entry: &entry}
		}
		return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: mainSessionKey}
	}

	trimmed := ""
	if heartbeat != nil && heartbeat.Session != nil {
		trimmed = strings.TrimSpace(*heartbeat.Session)
	}
	if trimmed == "" || strings.EqualFold(trimmed, "main") || strings.EqualFold(trimmed, "global") {
		if hasMain {
			entry := mainEntry
			return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: mainSessionKey, Entry: &entry}
		}
		return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: mainSessionKey}
	}

	if strings.HasPrefix(trimmed, "!") {
		if entry, ok := store.Sessions[trimmed]; ok {
			copyEntry := entry
			return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: trimmed, Entry: &copyEntry}
		}
		return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: trimmed}
	}

	candidate := toAgentStoreSessionKey(resolvedAgent, trimmed, "")
	if cfg != nil && cfg.Session != nil {
		candidate = toAgentStoreSessionKey(resolvedAgent, trimmed, cfg.Session.MainKey)
	}
	canonical := canonicalizeMainSessionAlias(cfg, resolvedAgent, candidate)
	if canonical != sessionScopeGlobal {
		sessionAgent := resolveAgentIdFromSessionKey(canonical)
		if sessionAgent == resolvedAgent {
			if entry, ok := store.Sessions[canonical]; ok {
				copyEntry := entry
				return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: canonical, Entry: &copyEntry}
			}
			return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: canonical}
		}
	}

	if hasMain {
		entry := mainEntry
		return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: mainSessionKey, Entry: &entry}
	}
	return heartbeatSessionResolution{StoreRef: storeRef, SessionKey: mainSessionKey}
}

func (oc *AIClient) resolveHeartbeatMainSessionRef(agentID string) (sessionStoreRef, string) {
	cfg := (*Config)(nil)
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
	storeRef := sessionStoreRef{
		AgentID: storeAgentID,
		Path:    resolveSessionStorePath(cfg, storeAgentID),
	}
	return storeRef, mainSessionKey
}
