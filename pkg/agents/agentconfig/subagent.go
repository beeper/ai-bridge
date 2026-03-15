// Package agentconfig provides shared agent configuration types used across
// the agents and tools packages to avoid import cycles.
package agentconfig

import "slices"

// SubagentConfig configures default subagent behavior for an agent.
type SubagentConfig struct {
	Model       string   `json:"model,omitempty"`
	Thinking    string   `json:"thinking,omitempty"`
	AllowAgents []string `json:"allowAgents,omitempty"`
}

// CloneSubagentConfig returns a deep copy of the given config.
func CloneSubagentConfig(cfg *SubagentConfig) *SubagentConfig {
	if cfg == nil {
		return nil
	}
	out := &SubagentConfig{
		Model:    cfg.Model,
		Thinking: cfg.Thinking,
	}
	if len(cfg.AllowAgents) > 0 {
		out.AllowAgents = slices.Clone(cfg.AllowAgents)
	}
	return out
}
