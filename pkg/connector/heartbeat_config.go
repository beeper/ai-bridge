package connector

import (
	"strings"

	"github.com/beeper/ai-bridge/pkg/agents"
)

const (
	defaultHeartbeatTarget = "last"
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
	if base == nil && override == nil {
		return override
	}
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	merged := *base
	// Override with non-zero fields
	if strings.TrimSpace(override.Every) != "" {
		merged.Every = override.Every
	}
	if override.ActiveHours != nil {
		merged.ActiveHours = override.ActiveHours
	}
	if strings.TrimSpace(override.Model) != "" {
		merged.Model = override.Model
	}
	if strings.TrimSpace(override.Session) != "" {
		merged.Session = override.Session
	}
	if strings.TrimSpace(override.Target) != "" {
		merged.Target = override.Target
	}
	if strings.TrimSpace(override.To) != "" {
		merged.To = override.To
	}
	if strings.TrimSpace(override.Prompt) != "" {
		merged.Prompt = override.Prompt
	}
	if override.AckMaxChars > 0 {
		merged.AckMaxChars = override.AckMaxChars
	}
	if override.IncludeReasoning != nil {
		merged.IncludeReasoning = override.IncludeReasoning
	}
	return &merged
}

func isHeartbeatEnabledForAgent(cfg *Config, agentID string) bool {
	resolved := normalizeAgentID(agentID)
	if cfg == nil || cfg.Agents == nil {
		return false
	}
	if hasExplicitHeartbeatAgents(cfg) {
		for _, entry := range cfg.Agents.List {
			if entry.Heartbeat == nil {
				continue
			}
			if normalizeAgentID(entry.ID) == resolved {
				return true
			}
		}
		return false
	}
	return resolved == normalizeAgentID(agents.DefaultAgentID)
}

func resolveHeartbeatIntervalMs(cfg *Config, overrideEvery string, heartbeat *HeartbeatConfig) int64 {
	raw := strings.TrimSpace(overrideEvery)
	if raw == "" && heartbeat != nil {
		raw = strings.TrimSpace(heartbeat.Every)
	}
	if raw == "" && cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Heartbeat != nil {
		raw = strings.TrimSpace(cfg.Agents.Defaults.Heartbeat.Every)
	}
	if raw == "" {
		raw = agents.DefaultHeartbeatEvery
	}
	if strings.TrimSpace(raw) == "" {
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
	if heartbeat != nil && strings.TrimSpace(heartbeat.Prompt) != "" {
		return agents.ResolveHeartbeatPrompt(heartbeat.Prompt)
	}
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Heartbeat != nil {
		return agents.ResolveHeartbeatPrompt(cfg.Agents.Defaults.Heartbeat.Prompt)
	}
	return agents.ResolveHeartbeatPrompt("")
}

func resolveHeartbeatAckMaxChars(cfg *Config, heartbeat *HeartbeatConfig) int {
	if heartbeat != nil && heartbeat.AckMaxChars > 0 {
		return heartbeat.AckMaxChars
	}
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Heartbeat != nil && cfg.Agents.Defaults.Heartbeat.AckMaxChars > 0 {
		return cfg.Agents.Defaults.Heartbeat.AckMaxChars
	}
	return agents.DefaultMaxAckChars
}

func resolveHeartbeatTarget(cfg *Config, heartbeat *HeartbeatConfig) string {
	if heartbeat != nil && strings.TrimSpace(heartbeat.Target) != "" {
		return strings.TrimSpace(heartbeat.Target)
	}
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.Heartbeat != nil && strings.TrimSpace(cfg.Agents.Defaults.Heartbeat.Target) != "" {
		return strings.TrimSpace(cfg.Agents.Defaults.Heartbeat.Target)
	}
	return defaultHeartbeatTarget
}
