package connector

import (
	"strings"

	"github.com/beeper/ai-bridge/pkg/agents"
)

func hasExplicitHeartbeatAgents(cfg *Config) bool {
	if cfg == nil || cfg.Agents == nil {
		return false
	}
	for _, entry := range cfg.Agents.List {
		if entry.Heartbeat != nil {
			return true
		}
	}
	return false
}

func resolveHeartbeatConfig(cfg *Config, agentID string) *HeartbeatConfig {
	if cfg == nil || cfg.Agents == nil {
		return nil
	}
	defaults := cfg.Agents.Defaults
	var base *HeartbeatConfig
	if defaults != nil {
		base = defaults.Heartbeat
	}
	normalized := normalizeAgentID(agentID)
	var override *HeartbeatConfig
	for _, entry := range cfg.Agents.List {
		if normalizeAgentID(entry.ID) == normalized {
			override = entry.Heartbeat
			break
		}
	}
	if base == nil || override == nil {
		// If both are nil, either works; if only one is non-nil, return it.
		if override != nil {
			return override
		}
		return base
	}
	merged := *base
	// Override with explicitly provided fields (OpenClaw-style)
	if override.Every != nil {
		merged.Every = override.Every
	}
	if override.ActiveHours != nil {
		merged.ActiveHours = override.ActiveHours
	}
	if override.Model != nil {
		merged.Model = override.Model
	}
	if override.Session != nil {
		merged.Session = override.Session
	}
	if override.Target != nil {
		merged.Target = override.Target
	}
	if override.To != nil {
		merged.To = override.To
	}
	if override.Prompt != nil {
		merged.Prompt = override.Prompt
	}
	if override.AckMaxChars != nil {
		merged.AckMaxChars = override.AckMaxChars
	}
	if override.IncludeReasoning != nil {
		merged.IncludeReasoning = override.IncludeReasoning
	}
	return &merged
}

func isHeartbeatEnabledForAgent(cfg *Config, agentID string) bool {
	resolved := normalizeAgentID(agentID)
	defaultAgent := normalizeAgentID(agents.DefaultAgentID)
	if cfg == nil || cfg.Agents == nil {
		return resolved == defaultAgent
	}
	if !hasExplicitHeartbeatAgents(cfg) {
		return resolved == defaultAgent
	}
	for _, entry := range cfg.Agents.List {
		if entry.Heartbeat != nil && normalizeAgentID(entry.ID) == resolved {
			return true
		}
	}
	return false
}

func resolveHeartbeatIntervalMs(cfg *Config, overrideEvery string, heartbeat *HeartbeatConfig) int64 {
	raw := strings.TrimSpace(overrideEvery)
	if raw == "" && heartbeat != nil && heartbeat.Every != nil {
		raw = strings.TrimSpace(*heartbeat.Every)
	}
	if raw == "" && cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Heartbeat != nil && cfg.Agents.Defaults.Heartbeat.Every != nil {
		raw = strings.TrimSpace(*cfg.Agents.Defaults.Heartbeat.Every)
	}
	if raw == "" {
		raw = agents.DefaultHeartbeatEvery
	}
	if raw == "" {
		return 0
	}
	ms, err := parseDurationMs(raw, "m")
	if err != nil || ms <= 0 {
		return 0
	}
	return ms
}

func resolveHeartbeatPrompt(cfg *Config, heartbeat *HeartbeatConfig, agent *agents.AgentDefinition) string {
	if agent != nil && strings.TrimSpace(agent.HeartbeatPrompt) != "" {
		return agents.ResolveHeartbeatPrompt(agent.HeartbeatPrompt)
	}
	if heartbeat != nil && heartbeat.Prompt != nil {
		return agents.ResolveHeartbeatPrompt(*heartbeat.Prompt)
	}
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Heartbeat != nil && cfg.Agents.Defaults.Heartbeat.Prompt != nil {
		return agents.ResolveHeartbeatPrompt(*cfg.Agents.Defaults.Heartbeat.Prompt)
	}
	return agents.ResolveHeartbeatPrompt("")
}

func resolveHeartbeatAckMaxChars(cfg *Config, heartbeat *HeartbeatConfig) int {
	if heartbeat != nil && heartbeat.AckMaxChars != nil {
		return max(*heartbeat.AckMaxChars, 0)
	}
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Heartbeat != nil && cfg.Agents.Defaults.Heartbeat.AckMaxChars != nil {
		return max(*cfg.Agents.Defaults.Heartbeat.AckMaxChars, 0)
	}
	return agents.DefaultMaxAckChars
}
