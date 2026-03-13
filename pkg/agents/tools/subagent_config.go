package tools

import "github.com/beeper/agentremote/pkg/agents/agentconfig"

// SubagentConfig is an alias for the shared type to preserve API compatibility.
type SubagentConfig = agentconfig.SubagentConfig

// cloneSubagentConfig creates a deep copy of the given config.
func cloneSubagentConfig(cfg *SubagentConfig) *SubagentConfig {
	return agentconfig.CloneSubagentConfig(cfg)
}
