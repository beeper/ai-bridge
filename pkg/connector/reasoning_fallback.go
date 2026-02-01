package connector

import (
	"context"

	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

// responseWithRetryAndReasoningFallback wraps responseWithRetry with reasoning level fallback logic.
// If the response fails with a reasoning-related error, it retries with a lower reasoning level.
func (oc *AIClient) responseWithRetryAndReasoningFallback(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
	responseFn responseFunc,
	logLabel string,
) {
	// Track attempted reasoning levels to avoid infinite loops
	attemptedLevels := make(map[string]bool)
	originalLevel := oc.effectiveReasoningEffort(meta)
	currentLevel := originalLevel
	maxReasoningFallbacks := 3

	for i := 0; i < maxReasoningFallbacks; i++ {
		attemptedLevels[currentLevel] = true

		// Create a modified meta with the current reasoning level if different from original
		effectiveMeta := meta
		if currentLevel != originalLevel {
			// Clone meta and override reasoning effort
			metaCopy := *meta
			metaCopy.ReasoningEffort = currentLevel
			effectiveMeta = &metaCopy
			oc.log.Info().
				Str("original_level", originalLevel).
				Str("fallback_level", currentLevel).
				Msg("Retrying with lower reasoning level")
		}

		// Try the request with current reasoning level
		// We use a wrapper that can detect reasoning errors
		success := oc.tryResponseWithReasoningDetection(ctx, evt, portal, effectiveMeta, prompt, responseFn, logLabel)
		if success {
			return
		}

		// Check if we should try a lower reasoning level
		fallbackLevel := FallbackReasoningLevel(currentLevel)
		if fallbackLevel == "" || attemptedLevels[fallbackLevel] {
			// No more fallbacks available or already tried
			return
		}

		currentLevel = fallbackLevel
	}
}

// tryResponseWithReasoningDetection wraps responseWithRetry and returns true if successful,
// false if it failed due to a reasoning-related error (allowing retry with lower level).
func (oc *AIClient) tryResponseWithReasoningDetection(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
	responseFn responseFunc,
	logLabel string,
) bool {
	// For now, we'll use responseWithRetry directly.
	// The reasoning error detection is done within streamingResponse.
	// This returns true if the request completed (success or non-reasoning error).
	oc.responseWithRetry(ctx, evt, portal, meta, prompt, responseFn, logLabel)
	return true // For now, always return true - reasoning fallback will be refined
}
