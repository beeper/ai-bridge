package connector

import "github.com/beeper/ai-bridge/pkg/aimodels"

// Type and constant aliases so that in-package code can use short names.
type ModelAPI = aimodels.ModelAPI

const (
	ModelAPIResponses       = aimodels.ModelAPIResponses
	ModelAPIChatCompletions = aimodels.ModelAPIChatCompletions
)

func (oc *AIClient) resolveModelAPI(meta *PortalMetadata) aimodels.ModelAPI {
	modelID := oc.effectiveModel(meta)
	if info := oc.findModelInfo(modelID); info != nil {
		if api := aimodels.NormalizeModelAPI(info.API); api != "" {
			if oc.isOpenRouterProvider() && api == aimodels.ModelAPIResponses {
				return aimodels.ModelAPIChatCompletions
			}
			return api
		}
	}
	if oc.isOpenRouterProvider() {
		return aimodels.ModelAPIChatCompletions
	}
	return aimodels.ModelAPIResponses
}
