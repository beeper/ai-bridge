package runtime

import "github.com/beeper/ai-bridge/pkg/matrixai/turnval"

// Re-export turn validation functions from the library package.
var (
	IsGoogleModel              = turnval.IsGoogleModel
	ValidateGeminiTurns        = turnval.ValidateGeminiTurns
	SanitizeGoogleTurnOrdering = turnval.SanitizeGoogleTurnOrdering
)
