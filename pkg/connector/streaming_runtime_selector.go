package connector

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/agents/tools"
	aipkg "github.com/beeper/ai-bridge/pkg/ai"
	airuntime "github.com/beeper/ai-bridge/pkg/runtime"
	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

type streamingRuntimePath string

const (
	streamingRuntimePkgAI           streamingRuntimePath = "pkg_ai"
	streamingRuntimeChatCompletions streamingRuntimePath = "chat_completions"
	streamingRuntimeResponses       streamingRuntimePath = "responses"
)

func pkgAIRuntimeEnabled() bool {
	return true
}

func chooseStreamingRuntimePath(hasAudio bool, _ ModelAPI, _ bool) streamingRuntimePath {
	if hasAudio {
		return streamingRuntimeChatCompletions
	}
	return streamingRuntimePkgAI
}

func (oc *AIClient) streamWithPkgAIBridge(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) (bool, *ContextLengthError, error) {
	aiContext := buildPkgAIContext(oc.effectivePrompt(meta), prompt)
	providerName := ""
	if loginMeta := loginMetadata(oc.UserLogin); loginMeta != nil {
		providerName = string(loginMeta.Provider)
	}
	aiModel := derivePkgAIModelDescriptor(
		oc.effectiveModel(meta),
		oc.effectiveModelForAPI(meta),
		providerName,
		oc.resolveModelAPI(meta),
		oc.effectiveMaxTokens(meta),
	)
	oc.loggerForContext(ctx).Debug().
		Int("prompt_messages", len(prompt)).
		Int("ai_messages", len(aiContext.Messages)).
		Str("ai_model_api", string(aiModel.API)).
		Str("ai_model_provider", string(aiModel.Provider)).
		Str("ai_model_id", aiModel.ID).
		Msg("Using pkg/ai runtime bridge as primary streaming path")

	baseURL, apiKey, ok := oc.pkgAIProviderBridgeCredentials()
	if !ok {
		return false, nil, errors.New("pkg/ai runtime requires OpenAI-compatible provider credentials")
	}

	params := oc.buildPkgAIBridgeGenerateParams(ctx, portal, meta, prompt)
	return oc.streamPkgAIBridgeEvents(ctx, evt, portal, meta, prompt, baseURL, apiKey, params)
}

func buildPkgAIContext(systemPrompt string, prompt []openai.ChatCompletionMessageParamUnion) aipkg.Context {
	unified := chatPromptToUnifiedMessages(prompt)
	return toAIContext(systemPrompt, unified, nil)
}

func chatPromptToUnifiedMessages(prompt []openai.ChatCompletionMessageParamUnion) []UnifiedMessage {
	out := make([]UnifiedMessage, 0, len(prompt))
	now := time.Now().UnixMilli()

	for _, msg := range prompt {
		switch {
		case msg.OfUser != nil:
			parts := make([]ContentPart, 0, 2)
			userText := strings.TrimSpace(airuntime.ExtractUserContent(msg.OfUser.Content))
			if userText != "" {
				parts = append(parts, ContentPart{Type: ContentTypeText, Text: userText})
			}
			for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
				if part.OfImageURL != nil && strings.TrimSpace(part.OfImageURL.ImageURL.URL) != "" {
					parts = append(parts, ContentPart{
						Type:     ContentTypeImage,
						ImageURL: strings.TrimSpace(part.OfImageURL.ImageURL.URL),
					})
				}
			}
			if len(parts) == 0 {
				continue
			}
			out = append(out, UnifiedMessage{
				Role:    RoleUser,
				Content: parts,
			})
		case msg.OfAssistant != nil:
			parts := make([]ContentPart, 0, 1)
			assistantText := strings.TrimSpace(airuntime.ExtractAssistantContent(msg.OfAssistant.Content))
			if assistantText != "" {
				parts = append(parts, ContentPart{Type: ContentTypeText, Text: assistantText})
			}
			toolCalls := make([]ToolCallResult, 0, len(msg.OfAssistant.ToolCalls))
			for _, toolCall := range msg.OfAssistant.ToolCalls {
				if toolCall.OfFunction == nil {
					continue
				}
				name := strings.TrimSpace(toolCall.OfFunction.Function.Name)
				if name == "" {
					continue
				}
				toolCalls = append(toolCalls, ToolCallResult{
					ID:        strings.TrimSpace(toolCall.OfFunction.ID),
					Name:      name,
					Arguments: strings.TrimSpace(toolCall.OfFunction.Function.Arguments),
				})
			}
			if len(parts) == 0 && len(toolCalls) == 0 {
				continue
			}
			out = append(out, UnifiedMessage{
				Role:      RoleAssistant,
				Content:   parts,
				ToolCalls: toolCalls,
			})
		case msg.OfTool != nil:
			toolText := strings.TrimSpace(airuntime.ExtractToolContent(msg.OfTool.Content))
			parts := []ContentPart{}
			if toolText != "" {
				parts = append(parts, ContentPart{Type: ContentTypeText, Text: toolText})
			}
			out = append(out, UnifiedMessage{
				Role:       RoleTool,
				ToolCallID: strings.TrimSpace(msg.OfTool.ToolCallID),
				Content:    parts,
			})
		case msg.OfSystem != nil || msg.OfDeveloper != nil:
			// System/developer content is carried separately via systemPrompt in buildPkgAIContext.
			continue
		default:
			content, role := airuntime.ExtractMessageContent(msg)
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			switch role {
			case "user":
				out = append(out, UnifiedMessage{Role: RoleUser, Content: []ContentPart{{Type: ContentTypeText, Text: content}}})
			case "assistant":
				out = append(out, UnifiedMessage{Role: RoleAssistant, Content: []ContentPart{{Type: ContentTypeText, Text: content}}})
			case "tool":
				out = append(out, UnifiedMessage{
					Role:       RoleTool,
					Content:    []ContentPart{{Type: ContentTypeText, Text: content}},
					ToolCallID: "tool_" + strconv.FormatInt(now, 10),
				})
			}
		}
	}
	return out
}

func (oc *AIClient) pkgAIProviderBridgeCredentials() (string, string, bool) {
	provider, ok := oc.provider.(*OpenAIProvider)
	if !ok || provider == nil {
		return "", "", false
	}
	return provider.baseURL, provider.apiKey, true
}

func (oc *AIClient) buildPkgAIBridgeGenerateParams(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) GenerateParams {
	return GenerateParams{
		Model:               oc.effectiveModel(meta),
		Messages:            chatPromptToUnifiedMessages(prompt),
		SystemPrompt:        oc.effectivePrompt(meta),
		Temperature:         oc.effectiveTemperature(meta),
		MaxCompletionTokens: oc.effectiveMaxTokens(meta),
		ReasoningEffort:     oc.effectiveReasoningEffort(meta),
		Tools:               oc.buildPkgAIBridgeTools(ctx, portal, meta),
	}
}

func (oc *AIClient) buildPkgAIBridgeTools(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) []ToolDefinition {
	definitions := append([]ToolDefinition(nil), oc.selectedBuiltinToolsForTurn(ctx, meta)...)
	if meta == nil || !meta.Capabilities.SupportsToolCalling {
		return dedupeToolDefinitionsByName(definitions)
	}

	hasAgent := resolveAgentID(meta) != ""
	if hasAgent && !hasBossAgent(meta) && !oc.isBuilderRoom(portal) {
		for _, tool := range tools.SessionTools() {
			if !oc.isToolEnabled(meta, tool.Name) {
				continue
			}
			definitions = append(definitions, toToolDefinitionFromAgentTool(tool))
		}
	}
	if hasBossAgent(meta) || oc.isBuilderRoom(portal) {
		for _, tool := range tools.BossTools() {
			if !oc.isToolEnabled(meta, tool.Name) {
				continue
			}
			definitions = append(definitions, toToolDefinitionFromAgentTool(tool))
		}
	}
	return dedupeToolDefinitionsByName(definitions)
}

func dedupeToolDefinitionsByName(tools []ToolDefinition) []ToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	deduped := make([]ToolDefinition, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		deduped = append(deduped, tool)
	}
	return deduped
}

func toToolDefinitionFromAgentTool(tool *tools.Tool) ToolDefinition {
	if tool == nil {
		return ToolDefinition{}
	}
	var parameters map[string]any
	switch schema := tool.InputSchema.(type) {
	case nil:
		parameters = nil
	case map[string]any:
		parameters = schema
	default:
		blob, err := json.Marshal(schema)
		if err == nil {
			_ = json.Unmarshal(blob, &parameters)
		}
		if len(parameters) == 0 {
			parameters = nil
		}
	}
	return ToolDefinition{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  parameters,
	}
}

func (oc *AIClient) streamPkgAIBridgeEvents(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
	baseURL string,
	apiKey string,
	params GenerateParams,
) (bool, *ContextLengthError, error) {
	log := oc.loggerForContext(ctx).With().
		Str("action", "stream_pkg_ai_bridge_events").
		Logger()

	prep, _, typingCleanup := oc.prepareStreamingRun(ctx, log, evt, portal, meta, prompt)
	defer typingCleanup()
	state := prep.State
	typingSignals := prep.TypingSignals
	touchTyping := prep.TouchTyping
	isHeartbeat := prep.IsHeartbeat

	oc.emitUIStart(ctx, portal, state, meta)
	currentParams := params
	const maxToolRounds = 10

	for round := 0; ; round++ {
		state.pendingFunctionOutputs = nil
		toolCallsThisRound := make([]ToolCallResult, 0, 4)
		activeTools := make(map[string]*activeToolCall)
		var roundContent strings.Builder
		events, handled := tryGenerateStreamWithPkgAI(ctx, baseURL, apiKey, currentParams)
		if !handled {
			err := errors.New("pkg/ai runtime stream was not handled by registered providers")
			oc.uiEmitter(state).EmitUIError(ctx, portal, err.Error())
			oc.emitUIFinish(ctx, portal, state, meta)
			return false, nil, streamFailureError(state, err)
		}

		for {
			select {
			case <-ctx.Done():
				state.finishReason = "cancelled"
				if state.hasInitialMessageTarget() && state.accumulated.Len() > 0 {
					oc.flushPartialStreamingMessage(context.Background(), portal, state, meta)
				}
				oc.uiEmitter(state).EmitUIAbort(ctx, portal, "cancelled")
				oc.emitUIFinish(ctx, portal, state, meta)
				return false, nil, streamFailureError(state, ctx.Err())
			case event, ok := <-events:
				if !ok {
					if shouldContinueChatToolLoop(state.finishReason, len(toolCallsThisRound)) {
						if round >= maxToolRounds {
							err := errors.New("max pkg/ai tool call rounds reached")
							oc.uiEmitter(state).EmitUIError(ctx, portal, err.Error())
							oc.emitUIFinish(ctx, portal, state, meta)
							return false, nil, streamFailureError(state, err)
						}

						assistantContent := make([]ContentPart, 0, 1)
						if text := strings.TrimSpace(roundContent.String()); text != "" {
							assistantContent = append(assistantContent, ContentPart{Type: ContentTypeText, Text: text})
						}
						currentParams.Messages = append(currentParams.Messages, UnifiedMessage{
							Role:      RoleAssistant,
							Content:   assistantContent,
							ToolCalls: toolCallsThisRound,
						})
						for _, output := range state.pendingFunctionOutputs {
							currentParams.Messages = append(currentParams.Messages, UnifiedMessage{
								Role:       RoleTool,
								ToolCallID: output.callID,
								Name:       output.name,
								Content: []ContentPart{
									{Type: ContentTypeText, Text: output.output},
								},
							})
						}
						state.pendingFunctionOutputs = nil
						state.needsTextSeparator = true

						if steerItems := oc.drainSteerQueue(state.roomID); len(steerItems) > 0 {
							for _, item := range steerItems {
								if item.pending.Type != pendingTypeText {
									continue
								}
								userPrompt := strings.TrimSpace(item.prompt)
								if userPrompt == "" {
									userPrompt = strings.TrimSpace(item.pending.MessageBody)
								}
								if userPrompt == "" {
									continue
								}
								currentParams.Messages = append(currentParams.Messages, UnifiedMessage{
									Role:    RoleUser,
									Content: []ContentPart{{Type: ContentTypeText, Text: userPrompt}},
								})
							}
						}
						goto nextRound
					}

					state.completedAtMs = time.Now().UnixMilli()
					oc.finalizeResponsesStream(ctx, log, portal, state, meta)
					return true, nil, nil
				}

				oc.markMessageSendSuccess(ctx, portal, evt, state)
				switch event.Type {
				case StreamEventDelta:
					touchTyping()
					roundContent.WriteString(event.Delta)
					if err := oc.handleResponseOutputTextDelta(
						ctx,
						log,
						portal,
						state,
						meta,
						typingSignals,
						isHeartbeat,
						event.Delta,
						"failed to send initial streaming message",
						"Failed to send initial streaming message",
					); err != nil {
						return false, nil, &PreDeltaError{Err: err}
					}
				case StreamEventReasoning:
					touchTyping()
					if err := oc.handleResponseReasoningTextDelta(
						ctx,
						log,
						portal,
						state,
						meta,
						isHeartbeat,
						event.ReasoningDelta,
						"failed to send initial streaming message",
						"Failed to send initial streaming message",
					); err != nil {
						return false, nil, &PreDeltaError{Err: err}
					}
				case StreamEventToolCall:
					if event.ToolCall == nil {
						continue
					}
					toolCallID := strings.TrimSpace(event.ToolCall.ID)
					if toolCallID == "" {
						toolCallID = NewCallID()
					}
					toolName := strings.TrimSpace(event.ToolCall.Name)
					arguments := normalizeToolArgsJSON(strings.TrimSpace(event.ToolCall.Arguments))
					toolCallsThisRound = append(toolCallsThisRound, ToolCallResult{
						ID:        toolCallID,
						Name:      toolName,
						Arguments: arguments,
					})
					oc.handleFunctionCallArgumentsDone(
						ctx,
						log,
						portal,
						state,
						meta,
						activeTools,
						toolCallID,
						toolName,
						arguments,
						true,
						" (pkg/ai)",
					)
				case StreamEventComplete:
					if reason := strings.TrimSpace(event.FinishReason); reason != "" {
						state.finishReason = reason
					}
					state.responseID = strings.TrimSpace(event.ResponseID)
					if event.Usage != nil {
						state.promptTokens = int64(event.Usage.PromptTokens)
						state.completionTokens = int64(event.Usage.CompletionTokens)
						state.reasoningTokens = int64(event.Usage.ReasoningTokens)
						state.totalTokens = int64(event.Usage.TotalTokens)
						oc.uiEmitter(state).EmitUIMessageMetadata(ctx, portal, oc.buildUIMessageMetadata(state, meta, true))
					}
				case StreamEventError:
					if cle := ParseContextLengthError(event.Error); cle != nil {
						return false, cle, nil
					}
					oc.uiEmitter(state).EmitUIError(ctx, portal, event.Error.Error())
					oc.emitUIFinish(ctx, portal, state, meta)
					return false, nil, streamFailureError(state, event.Error)
				}
			}
		}

	nextRound:
	}
}
