package connector

import "github.com/beeper/ai-bridge/pkg/core/aimodels"

type ModelAPI string

const (
	ModelAPIResponses       ModelAPI = ModelAPI(aimodels.ModelAPIResponses)
	ModelAPIChatCompletions ModelAPI = ModelAPI(aimodels.ModelAPIChatCompletions)
)

func (oc *AIClient) resolveModelAPI(meta *PortalMetadata) ModelAPI {
	modelID := oc.effectiveModel(meta)
	if info := oc.findModelInfo(modelID); info != nil {
		if api := aimodels.NormalizeModelAPI(info.API); api != "" {
			if oc.isOpenRouterProvider() && api == aimodels.ModelAPIResponses {
				return ModelAPIChatCompletions
			}
			return ModelAPI(api)
		}
	}
	if oc.isOpenRouterProvider() {
		return ModelAPIChatCompletions
	}
	return ModelAPIResponses
}
