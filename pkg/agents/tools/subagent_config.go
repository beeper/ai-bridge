package tools

// SubagentConfig mirrors OpenClaw-style subagent defaults for tools API payloads.
type SubagentConfig struct {
	Model       string   `json:"model,omitempty"`
	AllowAgents []string `json:"allowAgents,omitempty"`
}

func cloneSubagentConfig(cfg *SubagentConfig) *SubagentConfig {
	if cfg == nil {
		return nil
	}
	out := &SubagentConfig{
		Model: cfg.Model,
	}
	if len(cfg.AllowAgents) > 0 {
		out.AllowAgents = append([]string{}, cfg.AllowAgents...)
	}
	return out
}
