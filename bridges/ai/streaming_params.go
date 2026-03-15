package ai

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
)

// buildResponsesAPIParams creates common Responses API parameters for both streaming and non-streaming paths
func (oc *AIClient) buildResponsesAPIParams(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) responses.ResponseNewParams {
	log := zerolog.Ctx(ctx)

	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(oc.effectiveModelForAPI(meta)),
		MaxOutputTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		params.Instructions = openai.String(systemPrompt)
	}

	// Build full message history for every request.
	input := oc.convertToResponsesInput(messages, meta)
	params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: input,
	}

	// Add reasoning effort when the resolved target supports it.
	if reasoningEffort := oc.effectiveReasoningEffort(meta); reasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffort(reasoningEffort),
		}
	}

	// OpenRouter's Responses API only supports function-type tools.
	isOpenRouter := oc.isOpenRouterProvider()
	log.Debug().
		Bool("is_openrouter", isOpenRouter).
		Str("detected_provider", loginMetadata(oc.UserLogin).Provider).
		Msg("Provider detection for tool filtering")

	params.Tools = oc.selectedResponsesStreamingTools(ctx, meta, false)
	if len(params.Tools) > 0 {
		log.Debug().Int("count", len(params.Tools)).Msg("Added streaming turn tools")
	}

	// Prevent duplicate tool names (Anthropic rejects duplicates)
	logToolParamDuplicates(log, params.Tools)

	return params
}
