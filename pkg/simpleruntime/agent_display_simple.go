package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/simpledeps/agents"
)

func (oc *AIClient) resolveAgentDisplayName(_ context.Context, agent *agents.AgentDefinition) string {
	if agent == nil {
		return "Agent"
	}
	return agent.EffectiveName()
}

func (oc *AIClient) resolveAgentIdentityName(_ context.Context, _ string) string {
	return ""
}

func (oc *AIClient) agentDefaultModel(agent *agents.AgentDefinition) string {
	if agent != nil && agent.Model.Primary != "" {
		return ResolveAlias(agent.Model.Primary)
	}
	return oc.effectiveModel(nil)
}
