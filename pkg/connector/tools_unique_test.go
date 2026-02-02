package connector

import (
	"testing"

	agenttools "github.com/beeper/ai-bridge/pkg/agents/tools"
)

// Guard against provider errors like "tools: Tool names must be unique".
// Verifies uniqueness across builtin tools and boss tools we send to providers.
func TestToolNamesUnique(t *testing.T) {
	seen := make(map[string]string)

	// Builtin tools
	for _, tool := range BuiltinTools() {
		if tool.Name == "" {
			t.Fatalf("builtin tool has empty name: %+v", tool)
		}
		if prev, ok := seen[tool.Name]; ok {
			t.Fatalf("duplicate tool name %q between %s and builtin", tool.Name, prev)
		}
		seen[tool.Name] = "builtin"
	}

	// Boss tools
	for _, tool := range agenttools.BossTools() {
		if tool.Name == "" {
			t.Fatalf("boss tool has empty name: %+v", tool)
		}
		if prev, ok := seen[tool.Name]; ok {
			t.Fatalf("duplicate tool name %q between %s and boss", tool.Name, prev)
		}
		seen[tool.Name] = "boss"
	}
}
