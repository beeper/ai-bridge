package connector

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix/bridgev2"

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
	if agent.Model.Primary != "" {
		return agent.Model.Primary
	}
	return oc.defaultModelForProvider()
}

// ensureAgentModelGhostDisplayName updates the ghost's display name for an agent+model combination.
func (oc *AIClient) ensureAgentModelGhostDisplayName(ctx context.Context, agentID, modelID, agentName string) {
	userID := agentModelUserID(agentID, modelID)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return
	}

	displayName := oc.agentModelDisplayName(agentName, modelID)
	ghost.UpdateInfo(ctx, &bridgev2.UserInfo{
		Name:  &displayName,
		IsBot: ptrBool(true),
	})
}

func ptrBool(b bool) *bool {
	return &b
}
