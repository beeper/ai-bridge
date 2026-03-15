package ai

import (
	"context"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// buildContinuationParams builds params for continuing a response after tool execution
// and/or after responding to tool approval requests.
func (oc *AIClient) buildContinuationParams(
	ctx context.Context,
	state *streamingState,
	meta *PortalMetadata,
	pendingOutputs []functionCallOutput,
	approvalInputs []responses.ResponseInputItemUnionParam,
) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(oc.effectiveModelForAPI(meta)),
		MaxOutputTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	if systemPrompt := oc.effectivePrompt(meta); systemPrompt != "" {
		params.Instructions = openai.String(systemPrompt)
	}

	// Build function call outputs as input
	var input responses.ResponseInputParam
	if len(state.baseInput) > 0 {
		// All Responses continuations are stateless: include the accumulated local history.
		input = append(input, state.baseInput...)
	}
	input = append(input, approvalInputs...)
	for _, output := range pendingOutputs {
		if output.name != "" {
			args := output.arguments
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			input = append(input, responses.ResponseInputItemParamOfFunctionCall(args, output.callID, output.name))
		}
		input = append(input, buildFunctionCallOutputItem(output.callID, output.output, oc.isOpenRouterProvider()))
	}
	steerItems := oc.drainSteerQueue(state.roomID)
	if len(steerItems) > 0 {
		steerInput := oc.buildSteerInputItems(steerItems, meta)
		if len(steerInput) > 0 {
			input = append(input, steerInput...)
			if len(state.baseInput) > 0 {
				state.baseInput = append(state.baseInput, steerInput...)
			}
		}
	}
	params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: input,
	}

	// Add reasoning effort if configured
	if reasoningEffort := oc.effectiveReasoningEffort(meta); reasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffort(reasoningEffort),
		}
	}

	params.Tools = oc.selectedResponsesStreamingTools(ctx, meta, true)

	// Prevent duplicate tool names (Anthropic rejects duplicates)
	logToolParamDuplicates(&oc.log, params.Tools)

	return params
}

func (oc *AIClient) buildSteerInputItems(items []pendingQueueItem, meta *PortalMetadata) responses.ResponseInputParam {
	if oc == nil || len(items) == 0 {
		return nil
	}
	var input responses.ResponseInputParam
	for _, item := range items {
		if item.pending.Type != pendingTypeText {
			continue
		}
		prompt := strings.TrimSpace(item.prompt)
		if prompt == "" {
			prompt = item.pending.MessageBody
		}
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			continue
		}
		messages := []openai.ChatCompletionMessageParamUnion{openai.UserMessage(prompt)}
		input = append(input, oc.convertToResponsesInput(messages, meta)...)
	}
	return input
}
