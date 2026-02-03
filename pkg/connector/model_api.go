package connector

type ModelAPI string

const (
	ModelAPIResponses       ModelAPI = "responses"
	ModelAPIChatCompletions ModelAPI = "chat_completions"
)

func (oc *AIClient) resolveModelAPI(_ *PortalMetadata) ModelAPI {
	if oc.isOpenRouterProvider() {
		return ModelAPIChatCompletions
	}
	return ModelAPIResponses
}
