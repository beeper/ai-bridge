package connector

import (
	"context"
	"errors"
	"fmt"

	"github.com/beeper/ai-bridge/pkg/matrixai/aierrors"
	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

const (
	maxRetryAttempts = 3 // Maximum retry attempts for context length errors
)

// responseFunc is the signature for response handlers that can be retried on context length errors
type responseFunc func(ctx context.Context, evt *event.Event, portal *bridgev2.Portal, meta *PortalMetadata, prompt []openai.ChatCompletionMessageParamUnion) (bool, *aierrors.ContextLengthError, error)

// responseWithRetry wraps a response function with context length retry logic
// It first tries auto-compaction (LLM summarization) before falling back to reactive truncation
func (oc *AIClient) responseWithRetry(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
	responseFn responseFunc,
	logLabel string,
) (bool, error) {
	currentPrompt := prompt

	for attempt := range maxRetryAttempts {
		success, cle, err := responseFn(ctx, evt, portal, meta, currentPrompt)
		if success {
			return true, nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return true, nil
			}
			oc.loggerForContext(ctx).Warn().Err(err).Int("attempt", attempt+1).Str("log_label", logLabel).Msg("Response attempt failed with error")
			return false, err
		}

		// If we got a context length error, try truncation
		if cle != nil {
			oc.loggerForContext(ctx).Info().Int("attempt", attempt+1).Int("requested_tokens", cle.RequestedTokens).Int("max_tokens", cle.ModelMaxTokens).Str("log_label", logLabel).Msg("Context length exceeded, attempting recovery")
			// In Responses conversation mode, previous_response_id can accumulate hidden server-side
			// context that local truncation cannot affect. Reset it once and retry with local history.
			if meta != nil && meta.ConversationMode == "responses" && meta.LastResponseID != "" && !oc.isOpenRouterProvider() {
				oc.loggerForContext(ctx).Warn().
					Str("last_response_id", meta.LastResponseID).
					Msg("Context overflow in responses mode; clearing previous_response_id and retrying with local context")
				meta.LastResponseID = ""
				oc.savePortalQuiet(ctx, portal, "responses context reset")
				continue
			}

			// Reactive truncation
			truncated := oc.truncatePrompt(currentPrompt)
			if len(truncated) <= 2 {
				return false, cle
			}

			oc.notifyContextLengthExceeded(ctx, portal, cle, true)
			currentPrompt = truncated

			oc.loggerForContext(ctx).Debug().
				Int("attempt", attempt+1).
				Int("new_prompt_len", len(currentPrompt)).
				Str("log_label", logLabel).
				Msg("Retrying with truncated context")
			continue
		}

		// Non-context error, already handled in responseFn
		return false, nil
	}

	oc.notifyMatrixSendFailure(ctx, portal, evt,
		errors.New("exceeded maximum retry attempts for prompt overflow"))
	return false, nil
}

func (oc *AIClient) streamingResponseWithRetry(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	selector := func(meta *PortalMetadata, prompt []openai.ChatCompletionMessageParamUnion) (responseFunc, string) {
		return oc.selectResponseFn(meta, prompt)
	}
	oc.responseWithModelFallbackDynamic(ctx, evt, portal, meta, prompt, selector)
}

func (oc *AIClient) selectResponseFn(meta *PortalMetadata, prompt []openai.ChatCompletionMessageParamUnion) (responseFunc, string) {
	// Use Chat Completions API for audio (native support)
	// SDK v3.16.0 has ResponseInputAudioParam but it's not wired into the union
	if hasAudioContent(prompt) {
		return oc.streamChatCompletions, "chat_completions"
	}
	switch oc.resolveModelAPI(meta) {
	case ModelAPIChatCompletions:
		return oc.streamChatCompletions, "chat_completions"
	default:
		// Use Responses API for other content (images, files, text)
		return oc.streamingResponseWithToolSchemaFallback, "responses"
	}
}

// notifyContextLengthExceeded sends a user-friendly notice about context overflow
func (oc *AIClient) notifyContextLengthExceeded(
	ctx context.Context,
	portal *bridgev2.Portal,
	cle *aierrors.ContextLengthError,
	willRetry bool,
) {
	var message string
	if willRetry {
		message = fmt.Sprintf(
			"Your conversation exceeded the model's context limit (%d tokens requested, %d max). "+
				"Automatically trimming older messages and retrying...",
			cle.RequestedTokens, cle.ModelMaxTokens,
		)
	} else {
		message = fmt.Sprintf(
			"Your message is too long for this model's context window (%d tokens max). "+
				"Try a shorter message, or start a new conversation.",
			cle.ModelMaxTokens,
		)
	}

	oc.sendSystemNotice(ctx, portal, message)
}

// truncatePrompt intelligently prunes messages while preserving conversation coherence.
// Uses smart context pruning that:
// 1. Never removes system prompt or latest user message
// 2. First truncates large tool results (keeps head + tail)
// 3. Removes oldest messages while keeping tool call/result pairs together
// 4. Preserves recent context with higher priority
func (oc *AIClient) truncatePrompt(
	prompt []openai.ChatCompletionMessageParamUnion,
) []openai.ChatCompletionMessageParamUnion {
	// Use smart truncation with 50% reduction target
	return smartTruncatePrompt(prompt, 0.5)
}

