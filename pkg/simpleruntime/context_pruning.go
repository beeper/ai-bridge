package connector

import (
	"github.com/beeper/ai-bridge/pkg/matrixai/pruning"
	"github.com/openai/openai-go/v3"
)

type PruningConfig pruning.Config

func DefaultPruningConfig() *PruningConfig {
	return (*PruningConfig)(pruning.DefaultConfig())
}

func PruneContext(prompt []openai.ChatCompletionMessageParamUnion, config *PruningConfig, contextWindowTokens int) []openai.ChatCompletionMessageParamUnion {
	return pruning.PruneContext(prompt, (*pruning.Config)(config), contextWindowTokens)
}

func LimitHistoryTurns(prompt []openai.ChatCompletionMessageParamUnion, limit int) []openai.ChatCompletionMessageParamUnion {
	return pruning.LimitHistoryTurns(prompt, limit)
}

func applyPruningDefaults(config *PruningConfig) *PruningConfig {
	return (*PruningConfig)(pruning.ApplyDefaults((*pruning.Config)(config)))
}

// smartTruncatePrompt delegates to the library.
var smartTruncatePrompt = pruning.SmartTruncatePrompt

// estimateMessageChars delegates to the library.
var estimateMessageChars = pruning.EstimateMessageChars
