package connector

import "github.com/beeper/ai-bridge/pkg/matrixai/pruning"

// PruningConfig is an alias for the library pruning config.
type PruningConfig = pruning.Config

// Re-export pruning functions from the library package.
var (
	DefaultPruningConfig = pruning.DefaultConfig
	PruneContext         = pruning.PruneContext
	LimitHistoryTurns    = pruning.LimitHistoryTurns
)

// applyPruningDefaults delegates to the library.
var applyPruningDefaults = pruning.ApplyDefaults

// smartTruncatePrompt delegates to the library.
var smartTruncatePrompt = pruning.SmartTruncatePrompt

// estimateMessageChars delegates to the library.
var estimateMessageChars = pruning.EstimateMessageChars
