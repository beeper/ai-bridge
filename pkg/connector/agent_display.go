package connector

import "github.com/beeper/ai-bridge/pkg/agents"

// agentModelDisplayName returns a display name for an agent.
// Example: "Beep"
func (oc *AIClient) agentModelDisplayName(agentName, modelID string) string {
	return agentName
}

// agentDefaultModel returns the default model for an agent.
func (oc *AIClient) agentDefaultModel(agent *agents.AgentDefinition) string {
	if agent == nil {
		return oc.effectiveModel(nil)
	}
	if agent.Model.Primary != "" {
		return ResolveAlias(agent.Model.Primary)
	}
	return oc.effectiveModel(nil)
}
