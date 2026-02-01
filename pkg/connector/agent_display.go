package connector

import "fmt"

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
