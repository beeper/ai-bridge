package aimodels

import "strings"

// ModelAPI represents the API style to use for a model.
type ModelAPI string

const (
	ModelAPIResponses       ModelAPI = "responses"
	ModelAPIChatCompletions ModelAPI = "chat_completions"
)

// NormalizeModelAPI normalises a free-form API string into a ModelAPI constant.
func NormalizeModelAPI(value string) ModelAPI {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "responses", "openai-responses", "openai_responses":
		return ModelAPIResponses
	case "chat_completions", "chat-completions", "openai-completions", "openai_completions":
		return ModelAPIChatCompletions
	default:
		return ""
	}
}
