package connector

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// buildResponsesAPIParams creates common Responses API parameters for both streaming and non-streaming paths
func (oc *AIClient) buildResponsesAPIParams(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) responses.ResponseNewParams {
	log := zerolog.Ctx(ctx)

	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(oc.effectiveModelForAPI(meta)),
		MaxOutputTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	systemPrompt := oc.effectivePrompt(meta)

	// Use previous_response_id if in "responses" mode and ID exists.
	// OpenRouter's Responses API is stateless, so always send full history there.
	usePreviousResponse := meta.ConversationMode == "responses" && meta.LastResponseID != "" && !oc.isOpenRouterProvider()
	if usePreviousResponse {
		params.PreviousResponseID = openai.String(meta.LastResponseID)
		if systemPrompt != "" {
			params.Instructions = openai.String(systemPrompt)
		}
		// Still need to pass the latest user message as input
		if len(messages) > 0 {
			latestMsg := messages[len(messages)-1]
			input := oc.convertToResponsesInput([]openai.ChatCompletionMessageParamUnion{latestMsg}, meta)
			params.Input = responses.ResponseNewParamsInputUnion{
				OfInputItemList: input,
			}
		}
		log.Debug().Str("previous_response_id", meta.LastResponseID).Msg("Using previous_response_id for context")
	} else {
		// Build full message history
		input := oc.convertToResponsesInput(messages, meta)
		params.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		}
	}

	// Add reasoning effort if configured (uses inheritance: room → user → default)
	if reasoningEffort := oc.effectiveReasoningEffort(meta); reasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffort(reasoningEffort),
		}
	}

	isOpenRouter := oc.isOpenRouterProvider()
	hasAgent := resolveAgentID(meta) != ""
	strictMode := resolveToolStrictMode(isOpenRouter)

	// Add builtin function tools for this turn.
	// In simple mode this is intentionally restricted to web_search.
	enabledTools := oc.selectedBuiltinToolsForTurn(ctx, meta)
	if len(enabledTools) > 0 {
		params.Tools = append(params.Tools, ToOpenAITools(enabledTools, strictMode, &oc.log)...)
	}

	if meta.Capabilities.SupportsToolCalling && hasAgent {
		if !hasBossAgent(meta) && !oc.isBuilderRoom(portal) {
			if enabledSessions := oc.filterEnabledTools(meta, tools.SessionTools()); len(enabledSessions) > 0 {
				params.Tools = append(params.Tools, bossToolsToOpenAI(enabledSessions, strictMode, &oc.log)...)
			}
		}
	}

	if hasBossAgent(meta) || oc.isBuilderRoom(portal) {
		enabledBoss := oc.filterEnabledTools(meta, tools.BossTools())
		params.Tools = append(params.Tools, bossToolsToOpenAI(enabledBoss, strictMode, &oc.log)...)
	}

	if isOpenRouter {
		params.Tools = renameWebSearchToolParams(params.Tools)
	}

	// Prevent duplicate tool names (Anthropic rejects duplicates)
	logToolParamDuplicates(log, params.Tools)
	params.Tools = dedupeToolParams(params.Tools)

	return params
}

// filterEnabledTools returns the subset of tools that are enabled for this room/metadata.
func (oc *AIClient) filterEnabledTools(meta *PortalMetadata, all []*tools.Tool) []*tools.Tool {
	var enabled []*tools.Tool
	for _, tool := range all {
		if oc.isToolEnabled(meta, tool.Name) {
			enabled = append(enabled, tool)
		}
	}
	return enabled
}

// sanitizeToolSchema converts a tool's InputSchema to a sanitized map[string]any.
func sanitizeToolSchema(inputSchema any, log *zerolog.Logger, toolName string) map[string]any {
	var schema map[string]any
	switch v := inputSchema.(type) {
	case nil:
		return nil
	case map[string]any:
		schema = v
	default:
		encoded, err := json.Marshal(v)
		if err == nil {
			if err := json.Unmarshal(encoded, &schema); err != nil {
				return nil
			}
		}
	}
	if schema != nil {
		var stripped []string
		schema, stripped = sanitizeToolSchemaWithReport(schema)
		logSchemaSanitization(log, toolName, stripped)
	}
	return schema
}

// bossToolsToOpenAI converts boss tools to OpenAI Responses API format.
func bossToolsToOpenAI(bossTools []*tools.Tool, strictMode ToolStrictMode, log *zerolog.Logger) []responses.ToolUnionParam {
	var result []responses.ToolUnionParam
	for _, t := range bossTools {
		schema := sanitizeToolSchema(t.InputSchema, log, t.Name)
		toolParam := responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:       t.Name,
				Parameters: schema,
				Strict:     param.NewOpt(shouldUseStrictMode(strictMode, schema)),
				Type:       constant.ValueOf[constant.Function](),
			},
		}
		if t.Description != "" {
			toolParam.OfFunction.Description = openai.String(t.Description)
		}
		result = append(result, toolParam)
	}
	return result
}

// bossToolsToChatTools converts boss tools to OpenAI Chat Completions tool format.
func bossToolsToChatTools(bossTools []*tools.Tool, log *zerolog.Logger) []openai.ChatCompletionToolUnionParam {
	var result []openai.ChatCompletionToolUnionParam
	for _, t := range bossTools {
		schema := sanitizeToolSchema(t.InputSchema, log, t.Name)
		function := openai.FunctionDefinitionParam{
			Name:       t.Name,
			Parameters: schema,
		}
		if t.Description != "" {
			function.Description = openai.String(t.Description)
		}
		result = append(result, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: function,
				Type:     constant.ValueOf[constant.Function](),
			},
		})
	}
	return result
}
