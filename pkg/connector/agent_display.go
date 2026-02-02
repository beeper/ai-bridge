package connector

import (
	"fmt"

	"github.com/beeper/ai-bridge/pkg/agents"
)

// agentModelDisplayName returns a display name for an agent+model combination.
// Example: "Beeper AI (Claude Sonnet)"
func (oc *AIClient) agentModelDisplayName(agentName, modelID string) string {
	modelInfo := oc.findModelInfo(modelID)
	modelName := modelID
	if modelInfo != nil && modelInfo.Name != "" {
		modelName = modelInfo.Name
	} else {
		// Try to extract a readable name from the model ID
		_, actualModel := ParseModelPrefix(modelID)
		if actualModel != "" {
			modelName = actualModel
		}
	}
	return fmt.Sprintf("%s (%s)", agentName, modelName)
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
