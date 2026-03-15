package ai

import "github.com/beeper/agentremote/pkg/agents/agentconfig"

// subagentsToTools converts an agents-package SubagentConfig to a tools-package one.
// Both are now aliases for agentconfig.SubagentConfig, so this is an identity function
// kept for call-site clarity.
func subagentsToTools(cfg *agentconfig.SubagentConfig) *agentconfig.SubagentConfig {
	return cfg
}

// subagentsFromTools converts a tools-package SubagentConfig to an agents-package one.
// Both are now aliases for agentconfig.SubagentConfig, so this is an identity function
// kept for call-site clarity.
func subagentsFromTools(cfg *agentconfig.SubagentConfig) *agentconfig.SubagentConfig {
	return cfg
}
