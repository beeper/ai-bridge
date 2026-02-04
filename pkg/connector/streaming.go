package connector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// streamingState tracks the state of a streaming response
type streamingState struct {
	turnID         string
	agentID        string
	startedAtMs    int64
	firstTokenAtMs int64
	completedAtMs  int64

	promptTokens     int64
	completionTokens int64
	reasoningTokens  int64
	totalTokens      int64

	baseInput              responses.ResponseInputParam
	accumulated            strings.Builder
	reasoning              strings.Builder
	toolCalls              []ToolCallMetadata
	pendingImages          []generatedImage
	pendingFunctionOutputs []functionCallOutput // Function outputs to send back to API for continuation
	initialEventID         id.EventID
	finishReason           string
	responseID             string
	sequenceNum            int
	firstToken             bool

	// Directive processing
	sourceEventID id.EventID // The triggering user message event ID (for [[reply_to_current]])

	// Heartbeat handling
	heartbeat         *HeartbeatRunConfig
	heartbeatResultCh chan HeartbeatRunOutcome
	suppressSave      bool
	suppressSend      bool
}

// newStreamingState creates a new streaming state with initialized fields
func newStreamingState(ctx context.Context, meta *PortalMetadata, sourceEventID id.EventID) *streamingState {
	state := &streamingState{
		turnID:        NewTurnID(),
		agentID:       meta.DefaultAgentID,
		startedAtMs:   time.Now().UnixMilli(),
		firstToken:    true,
		sourceEventID: sourceEventID,
	}
	if hb := heartbeatRunFromContext(ctx); hb != nil {
		state.heartbeat = hb.Config
		state.heartbeatResultCh = hb.ResultCh
		if hb.Config != nil && hb.Config.SuppressSave {
			state.suppressSave = true
		}
		if hb.Config != nil && hb.Config.SuppressSend {
			state.suppressSend = true
		}
	}
	return state
}

// generatedImage tracks a pending image from image generation
type generatedImage struct {
	itemID   string
	imageB64 string
	turnID   string
}

// functionCallOutput tracks a completed function call output for API continuation
type functionCallOutput struct {
	callID    string // The ItemID from the stream event (used as call_id in continuation)
	name      string // Tool name (for stateless continuations)
	arguments string // Raw arguments JSON (for stateless continuations)
	output    string // The result from executing the tool
}

func buildFunctionCallOutputItem(callID, output string, includeID bool) responses.ResponseInputItemUnionParam {
	item := responses.ResponseInputItemFunctionCallOutputParam{
		CallID: callID,
		Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
			OfString: param.NewOpt(output),
		},
	}
	if includeID {
		item.ID = param.NewOpt("fc_output_" + callID)
	}
	return responses.ResponseInputItemUnionParam{OfFunctionCallOutput: &item}
}

// saveAssistantMessage saves the completed assistant message to the database
func (oc *AIClient) saveAssistantMessage(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
) {
	modelID := oc.effectiveModel(meta)
	assistantMsg := &database.Message{
		ID:        MakeMessageID(state.initialEventID),
		Room:      portal.PortalKey,
		SenderID:  modelUserID(modelID),
		MXID:      state.initialEventID,
		Timestamp: time.Now(),
		Metadata: &MessageMetadata{
			Role:           "assistant",
			Body:           state.accumulated.String(),
			CompletionID:   state.responseID,
			FinishReason:   state.finishReason,
			Model:          modelID,
			TurnID:         state.turnID,
			AgentID:        state.agentID,
			ToolCalls:      state.toolCalls,
			StartedAtMs:    state.startedAtMs,
			FirstTokenAtMs: state.firstTokenAtMs,
			CompletedAtMs:  state.completedAtMs,
			HasToolCalls:   len(state.toolCalls) > 0,
			// Reasoning fields (only populated by Responses API)
			ThinkingContent:    state.reasoning.String(),
			ThinkingTokenCount: len(strings.Fields(state.reasoning.String())),
			PromptTokens:       state.promptTokens,
			CompletionTokens:   state.completionTokens,
			ReasoningTokens:    state.reasoningTokens,
		},
	}
	if err := oc.UserLogin.Bridge.DB.Message.Insert(ctx, assistantMsg); err != nil {
		log.Warn().Err(err).Msg("Failed to save assistant message to database")
	} else {
		log.Debug().Str("msg_id", string(assistantMsg.ID)).Msg("Saved assistant message to database")
	}
	oc.notifySessionMemoryChange(ctx, portal, meta, false)

	// Save LastResponseID for "responses" mode context chaining (OpenAI-only)
	if meta.ConversationMode == "responses" && state.responseID != "" && !oc.isOpenRouterProvider() {
		meta.LastResponseID = state.responseID
		if err := portal.Save(ctx); err != nil {
			log.Warn().Err(err).Msg("Failed to save portal after storing response ID")
		}
	}
}

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

	// OpenRouter's Responses API only supports function-type tools.
	isOpenRouter := oc.isOpenRouterProvider()
	log.Debug().
		Bool("is_openrouter", isOpenRouter).
		Str("detected_provider", loginMetadata(oc.UserLogin).Provider).
		Msg("Provider detection for tool filtering")

	// Add builtin function tools if model supports tool calling
	if meta.Capabilities.SupportsToolCalling {
		enabledTools := GetEnabledBuiltinTools(func(name string) bool {
			return oc.isToolEnabled(meta, name)
		})
		if len(enabledTools) > 0 {
			strictMode := resolveToolStrictMode(oc.isOpenRouterProvider())
			params.Tools = append(params.Tools, ToOpenAITools(enabledTools, strictMode, &oc.log)...)
			log.Debug().Int("count", len(enabledTools)).Msg("Added builtin function tools")
		}

		// Add session tools for non-boss rooms
		if !hasBossAgent(meta) && !oc.isBuilderRoom(portal) {
			var enabledSessions []*tools.Tool
			for _, tool := range tools.SessionTools() {
				if oc.isToolEnabled(meta, tool.Name) {
					enabledSessions = append(enabledSessions, tool)
				}
			}
			if len(enabledSessions) > 0 {
				strictMode := resolveToolStrictMode(oc.isOpenRouterProvider())
				params.Tools = append(params.Tools, bossToolsToOpenAI(enabledSessions, strictMode, &oc.log)...)
				log.Debug().Int("count", len(enabledSessions)).Msg("Added session tools")
			}
		}
	}

	// Add boss tools if this is a Boss room
	if hasBossAgent(meta) || oc.isBuilderRoom(portal) {
		var enabledBoss []*tools.Tool
		for _, tool := range tools.BossTools() {
			if oc.isToolEnabled(meta, tool.Name) {
				enabledBoss = append(enabledBoss, tool)
			}
		}
		strictMode := resolveToolStrictMode(oc.isOpenRouterProvider())
		params.Tools = append(params.Tools, bossToolsToOpenAI(enabledBoss, strictMode, &oc.log)...)
		log.Debug().Int("count", len(enabledBoss)).Msg("Added boss agent tools")
	}

	if oc.isOpenRouterProvider() {
		params.Tools = renameWebSearchToolParams(params.Tools)
	}

	// Prevent duplicate tool names (Anthropic rejects duplicates)
	params.Tools = dedupeToolParams(params.Tools)
	logToolParamDuplicates(log, params.Tools)

	return params
}

// bossToolsToOpenAI converts boss tools to OpenAI Responses API format.
func bossToolsToOpenAI(bossTools []*tools.Tool, strictMode ToolStrictMode, log *zerolog.Logger) []responses.ToolUnionParam {
	var result []responses.ToolUnionParam
	for _, t := range bossTools {
		var schema map[string]any
		switch v := t.InputSchema.(type) {
		case nil:
			schema = nil
		case map[string]any:
			schema = v
		default:
			encoded, err := json.Marshal(v)
			if err == nil {
				if err := json.Unmarshal(encoded, &schema); err != nil {
					schema = nil
				}
			}
		}
		if schema != nil {
			var stripped []string
			schema, stripped = sanitizeToolSchemaWithReport(schema)
			logSchemaSanitization(log, t.Name, stripped)
		}
		strict := shouldUseStrictMode(strictMode, schema)
		toolParam := responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:       t.Name,
				Parameters: schema,
				Strict:     param.NewOpt(strict),
				Type:       constant.ValueOf[constant.Function](),
			},
		}
		if t.Description != "" && toolParam.OfFunction != nil {
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
		var schema map[string]any
		switch v := t.InputSchema.(type) {
		case nil:
			schema = nil
		case map[string]any:
			schema = v
		default:
			encoded, err := json.Marshal(v)
			if err == nil {
				if err := json.Unmarshal(encoded, &schema); err != nil {
					schema = nil
				}
			}
		}
		if schema != nil {
			var stripped []string
			schema, stripped = sanitizeToolSchemaWithReport(schema)
			logSchemaSanitization(log, t.Name, stripped)
		}
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

// streamingResponseWithToolSchemaFallback retries via Chat Completions when the provider
// rejects tool schemas in the Responses API.
func (oc *AIClient) streamingResponseWithToolSchemaFallback(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	messages []openai.ChatCompletionMessageParamUnion,
) (bool, *ContextLengthError, error) {
	success, cle, err := oc.streamingResponse(ctx, evt, portal, meta, messages)
	if success || cle != nil || err == nil {
		return success, cle, err
	}
	if IsToolUniquenessError(err) {
		oc.log.Warn().Err(err).Msg("Duplicate tool names rejected; retrying with chat completions")
		success, cle, chatErr := oc.streamChatCompletions(ctx, evt, portal, meta, messages)
		if success || cle != nil || chatErr == nil {
			return success, cle, chatErr
		}
		if IsToolSchemaError(chatErr) || IsToolUniquenessError(chatErr) {
			oc.log.Warn().Err(chatErr).Msg("Chat completions tools rejected; retrying without tools")
			if meta != nil {
				metaCopy := *meta
				metaCopy.Capabilities = meta.Capabilities
				metaCopy.Capabilities.SupportsToolCalling = false
				return oc.streamChatCompletions(ctx, evt, portal, &metaCopy, messages)
			}
		}
		return success, cle, chatErr
	}
	if IsToolSchemaError(err) {
		oc.log.Warn().Err(err).Msg("Responses tool schema rejected; falling back to chat completions")
		success, cle, chatErr := oc.streamChatCompletions(ctx, evt, portal, meta, messages)
		if success || cle != nil || chatErr == nil {
			return success, cle, chatErr
		}
		if IsToolSchemaError(chatErr) {
			oc.log.Warn().Err(chatErr).Msg("Chat completions tool schema rejected; retrying without tools")
			if meta != nil {
				metaCopy := *meta
				metaCopy.Capabilities = meta.Capabilities
				metaCopy.Capabilities.SupportsToolCalling = false
				return oc.streamChatCompletions(ctx, evt, portal, &metaCopy, messages)
			}
		}
		return success, cle, chatErr
	}
	if IsNoResponseChunksError(err) {
		oc.log.Warn().Err(err).Msg("Responses streaming returned no chunks; retrying without tools")
		if meta != nil && meta.Capabilities.SupportsToolCalling {
			metaCopy := *meta
			metaCopy.Capabilities = meta.Capabilities
			metaCopy.Capabilities.SupportsToolCalling = false
			success, cle, retryErr := oc.streamingResponse(ctx, evt, portal, &metaCopy, messages)
			if success || cle != nil || retryErr == nil {
				return success, cle, retryErr
			}
			err = retryErr
		}
		oc.log.Warn().Err(err).Msg("Responses retry failed; falling back to chat completions")
		return oc.streamChatCompletions(ctx, evt, portal, meta, messages)
	}
	return success, cle, err
}

// streamingResponse handles streaming using the Responses API
// This is the preferred streaming method as it supports reasoning tokens
// Returns (success, contextLengthError)
func (oc *AIClient) streamingResponse(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	messages []openai.ChatCompletionMessageParamUnion,
) (bool, *ContextLengthError, error) {
	log := zerolog.Ctx(ctx).With().
		Str("portal_id", string(portal.ID)).
		Logger()

	// Initialize streaming state with turn tracking
	// Pass source event ID for [[reply_to_current]] directive support
	var sourceEventID id.EventID
	if evt != nil {
		sourceEventID = evt.ID
	}
	state := newStreamingState(ctx, meta, sourceEventID)

	// Ensure model ghost is in the room before any operations
	if !state.suppressSend {
		if err := oc.ensureModelInRoom(ctx, portal); err != nil {
			log.Warn().Err(err).Msg("Failed to ensure model is in room")
			// Continue anyway - typing will fail gracefully
		}
	}

	// Create typing controller with TTL and automatic refresh
	var typingCtrl *TypingController
	touchTyping := func() {}
	if !state.suppressSend && state.heartbeat == nil {
		typingCtrl = NewTypingController(oc, ctx, portal)
		typingCtrl.Start()
		defer typingCtrl.Stop()
		touchTyping = func() {
			typingCtrl.RefreshTTL()
		}
	}

	// Apply proactive context pruning if enabled
	messages = oc.applyProactivePruning(ctx, messages, meta)

	// Build Responses API params using shared helper
	params := oc.buildResponsesAPIParams(ctx, portal, meta, messages)

	// Inject per-room PDF engine into context for OpenRouter/Beeper providers
	if oc.isOpenRouterProvider() {
		ctx = WithPDFEngine(ctx, oc.effectivePDFEngine(meta))
	}

	stream := oc.api.Responses.NewStreaming(ctx, params)
	if stream == nil {
		initErr := fmt.Errorf("responses streaming not available")
		logResponsesFailure(log, initErr, params, meta, messages, "stream_init")
		return false, nil, &PreDeltaError{Err: initErr}
	}

	// Store base input for OpenRouter stateless continuations
	if params.Input.OfInputItemList != nil {
		state.baseInput = params.Input.OfInputItemList
	}

	// Track active tool calls
	activeTools := make(map[string]*activeToolCall)

	// Emit generation status: starting
	oc.emitStatusEvent(ctx, portal, state, "starting", "Starting generation...", nil)

	// Process stream events - no debouncing, stream every delta immediately
	for stream.Next() {
		streamEvent := stream.Current()

		rawPayload := &StreamEventRawPayload{Type: streamEvent.Type, JSON: streamEvent.RawJSON()}

		switch streamEvent.Type {
		case "response.output_text.delta":
			touchTyping()
			state.accumulated.WriteString(streamEvent.Delta)

			// First token - send initial message synchronously to capture event_id
			if state.firstToken && state.accumulated.Len() > 0 {
				state.firstToken = false
				state.firstTokenAtMs = time.Now().UnixMilli()
				if !state.suppressSend {
					// Ensure ghost display name is set before sending the first message
					oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
					state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID, state.sourceEventID)
					if state.initialEventID == "" {
						log.Error().Msg("Failed to send initial streaming message")
						return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message")}
					}
					// Update status to generating
					oc.emitStatusEvent(ctx, portal, state, "generating", "Generating response...", nil)
				}
			}

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "text_delta", map[string]any{
				"text": streamEvent.Delta,
			}, rawPayload)

		case "response.reasoning_text.delta":
			touchTyping()
			state.reasoning.WriteString(streamEvent.Delta)

			// Check if this is first content (reasoning before text)
			if state.firstToken && state.reasoning.Len() > 0 {
				state.firstToken = false
				state.firstTokenAtMs = time.Now().UnixMilli()
				if !state.suppressSend {
					oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
					// Send empty initial message - will be replaced with content later
					state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
					if state.initialEventID == "" {
						log.Error().Msg("Failed to send initial streaming message")
						return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message")}
					}
					// Update status to thinking
					oc.emitStatusEvent(ctx, portal, state, "thinking", "Thinking...", nil)
				}
			}

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "reasoning_delta", map[string]any{
				"text": streamEvent.Delta,
			}, rawPayload)

		case "response.refusal.delta":
			touchTyping()
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "refusal_delta", map[string]any{
				"text": streamEvent.Delta,
			}, rawPayload)

		case "response.function_call_arguments.delta":
			touchTyping()
			// Get or create active tool call
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				callID := streamEvent.ItemID
				if strings.TrimSpace(callID) == "" {
					callID = NewCallID()
				}
				tool = &activeToolCall{
					callID:      callID,
					toolName:    streamEvent.Name,
					toolType:    ToolTypeFunction,
					startedAtMs: time.Now().UnixMilli(),
					itemID:      streamEvent.ItemID,
				}
				activeTools[streamEvent.ItemID] = tool

				if state.initialEventID == "" && !state.suppressSend {
					oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
				}

				// Update status to tool_use
				oc.emitStatusEvent(ctx, portal, state, "tool_use", fmt.Sprintf("Calling %s...", streamEvent.Name), &GenerationDetails{
					CurrentTool: streamEvent.Name,
					CallID:      tool.callID,
				})
			}

			// Accumulate arguments
			tool.input.WriteString(streamEvent.Delta)

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_call_delta", map[string]any{
				"item_id":         streamEvent.ItemID,
				"name":            streamEvent.Name,
				"arguments_delta": streamEvent.Delta,
				"call_id":         tool.callID,
			}, rawPayload)

		case "response.function_call_arguments.done":
			touchTyping()
			// Function call complete - execute the tool and send result
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				// Create tool if we missed the delta events
				callID := streamEvent.ItemID
				if strings.TrimSpace(callID) == "" {
					callID = NewCallID()
				}
				tool = &activeToolCall{
					callID:      callID,
					toolName:    streamEvent.Name,
					toolType:    ToolTypeFunction,
					startedAtMs: time.Now().UnixMilli(),
				}
				tool.input.WriteString(streamEvent.Arguments)
				activeTools[streamEvent.ItemID] = tool
			}

			// Store the item ID for continuation (this is the call_id for the Responses API)
			tool.itemID = streamEvent.ItemID

			toolName := strings.TrimSpace(tool.toolName)
			if toolName == "" {
				toolName = strings.TrimSpace(streamEvent.Name)
			}
			tool.toolName = toolName
			argsJSON := strings.TrimSpace(tool.input.String())
			if argsJSON == "" {
				argsJSON = strings.TrimSpace(streamEvent.Arguments)
			}
			argsJSON = normalizeToolArgsJSON(argsJSON)

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_call_done", map[string]any{
				"item_id":   streamEvent.ItemID,
				"name":      toolName,
				"arguments": argsJSON,
				"call_id":   tool.callID,
			}, rawPayload)

			resultStatus := ResultStatusSuccess
			var result string
			if !oc.isToolEnabled(meta, toolName) {
				resultStatus = ResultStatusError
				result = fmt.Sprintf("Error: tool %s is disabled", toolName)
			} else {
				// Wrap context with bridge info for tools that need it (e.g., channel-edit, react)
				toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
					Client:        oc,
					Portal:        portal,
					Meta:          meta,
					SourceEventID: state.sourceEventID,
				})
				var err error
				result, err = oc.executeBuiltinTool(toolCtx, portal, toolName, argsJSON)
				if err != nil {
					log.Warn().Err(err).Str("tool", toolName).Msg("Tool execution failed")
					result = fmt.Sprintf("Error: %s", err.Error())
					resultStatus = ResultStatusError
				}
			}

			// Check for TTS audio result (AUDIO: prefix)
			displayResult := result
			if strings.HasPrefix(result, TTSResultPrefix) {
				audioB64 := strings.TrimPrefix(result, TTSResultPrefix)
				audioData, err := base64.StdEncoding.DecodeString(audioB64)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to decode TTS audio")
					displayResult = "Error: failed to decode TTS audio"
					resultStatus = ResultStatusError
				} else {
					// Send audio message
					if _, err := oc.sendGeneratedAudio(ctx, portal, audioData, "audio/mpeg", state.turnID); err != nil {
						log.Warn().Err(err).Msg("Failed to send TTS audio")
						displayResult = "Error: failed to send TTS audio"
						resultStatus = ResultStatusError
					} else {
						displayResult = "Audio message sent successfully"
					}
				}
				result = displayResult
			}

			// Check for image generation result (IMAGE: / IMAGES: prefix)
			if strings.HasPrefix(result, ImagesResultPrefix) {
				payload := strings.TrimPrefix(result, ImagesResultPrefix)
				var images []string
				if err := json.Unmarshal([]byte(payload), &images); err != nil {
					log.Warn().Err(err).Msg("Failed to parse generated images payload")
					displayResult = "Error: failed to parse generated images"
					resultStatus = ResultStatusError
				} else {
					success := 0
					for _, imageB64 := range images {
						imageData, mimeType, err := decodeBase64Image(imageB64)
						if err != nil {
							log.Warn().Err(err).Msg("Failed to decode generated image")
							continue
						}
						if _, err := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, state.turnID); err != nil {
							log.Warn().Err(err).Msg("Failed to send generated image")
							continue
						}
						success++
					}
					if success == len(images) && success > 0 {
						displayResult = fmt.Sprintf("Images generated and sent successfully (%d)", success)
					} else if success > 0 {
						displayResult = fmt.Sprintf("Images generated with %d/%d sent successfully", success, len(images))
						resultStatus = ResultStatusError
					} else {
						displayResult = "Error: failed to send generated images"
						resultStatus = ResultStatusError
					}
				}
				result = displayResult
			} else if strings.HasPrefix(result, ImageResultPrefix) {
				imageB64 := strings.TrimPrefix(result, ImageResultPrefix)
				imageData, mimeType, err := decodeBase64Image(imageB64)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to decode generated image")
					displayResult = "Error: failed to decode generated image"
					resultStatus = ResultStatusError
				} else {
					// Send image message
					if _, err := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, state.turnID); err != nil {
						log.Warn().Err(err).Msg("Failed to send generated image")
						displayResult = "Error: failed to send generated image"
						resultStatus = ResultStatusError
					} else {
						displayResult = "Image generated and sent successfully"
					}
				}
				result = displayResult
			}

			// Store result for API continuation
			tool.result = result
			args := argsJSON
			state.pendingFunctionOutputs = append(state.pendingFunctionOutputs, functionCallOutput{
				callID:    streamEvent.ItemID,
				name:      toolName,
				arguments: args,
				output:    result,
			})

			// Parse input for storage
			var inputMap map[string]any
			_ = json.Unmarshal([]byte(argsJSON), &inputMap)

			// Track tool call in metadata
			state.toolCalls = append(state.toolCalls, ToolCallMetadata{
				CallID:        tool.callID,
				ToolName:      toolName,
				ToolType:      string(tool.toolType),
				Input:         inputMap,
				Output:        map[string]any{"result": result},
				Status:        string(ToolStatusCompleted),
				ResultStatus:  string(resultStatus),
				StartedAtMs:   tool.startedAtMs,
				CompletedAtMs: time.Now().UnixMilli(),
			})

			toolStatus := ToolStatusCompleted
			if resultStatus != ResultStatusSuccess {
				toolStatus = ToolStatusFailed
			}
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceInternal, "tool_result", map[string]any{
				"call_id":       tool.callID,
				"tool_name":     toolName,
				"result":        result,
				"status":        string(toolStatus),
				"result_status": string(resultStatus),
			}, nil)

			// Update status back to generating
			oc.emitStatusEvent(ctx, portal, state, "generating", "Continuing generation...", nil)

		case "response.web_search_call.searching", "response.web_search_call.in_progress":
			touchTyping()
			// Web search starting
			callID := streamEvent.ItemID
			if strings.TrimSpace(callID) == "" {
				callID = NewCallID()
			}
			tool := &activeToolCall{
				callID:      callID,
				toolName:    "web_search",
				toolType:    ToolTypeProvider,
				startedAtMs: time.Now().UnixMilli(),
				itemID:      streamEvent.ItemID,
			}
			activeTools[streamEvent.ItemID] = tool

			if state.initialEventID == "" && !state.suppressSend {
				oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
			}

			// Update status
			oc.emitStatusEvent(ctx, portal, state, "tool_use", "Searching the web...", &GenerationDetails{
				CurrentTool: "web_search",
				CallID:      tool.callID,
			})

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_progress", map[string]any{
				"tool_name": "web_search",
				"status":    "searching",
				"call_id":   tool.callID,
				"progress": map[string]any{
					"message": "Searching...",
					"percent": 0,
				},
			}, rawPayload)

		case "response.web_search_call.completed":
			touchTyping()
			// Web search completed
			tool, exists := activeTools[streamEvent.ItemID]
			callID := ""
			if exists && tool != nil {
				callID = tool.callID
			}
			if callID == "" {
				callID = streamEvent.ItemID
			}
			if exists {
				// Track tool call
				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        callID,
					ToolName:      "web_search",
					ToolType:      string(tool.toolType),
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(ResultStatusSuccess),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
				})
			}

			// Update status back to generating
			oc.emitStatusEvent(ctx, portal, state, "generating", "Continuing generation...", nil)

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_progress", map[string]any{
				"tool_name": "web_search",
				"status":    "completed",
				"call_id":   callID,
				"progress": map[string]any{
					"message": "Search completed",
					"percent": 100,
				},
			}, rawPayload)

		case "response.image_generation_call.in_progress", "response.image_generation_call.generating":
			touchTyping()
			// Image generation in progress
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				callID := streamEvent.ItemID
				if strings.TrimSpace(callID) == "" {
					callID = NewCallID()
				}
				tool = &activeToolCall{
					callID:      callID,
					toolName:    "image_generation",
					toolType:    ToolTypeProvider,
					startedAtMs: time.Now().UnixMilli(),
					itemID:      streamEvent.ItemID,
				}
				activeTools[streamEvent.ItemID] = tool

				if state.initialEventID == "" && !state.suppressSend {
					oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
				}
			}

			// Update status
			oc.emitStatusEvent(ctx, portal, state, "image_generating", "Generating image...", &GenerationDetails{
				CurrentTool: "image_generation",
				CallID:      tool.callID,
			})

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_progress", map[string]any{
				"tool_name": "image_generation",
				"status":    "generating",
				"call_id":   tool.callID,
				"progress": map[string]any{
					"message": "Generating image...",
					"percent": 50,
				},
			}, rawPayload)

			log.Debug().Str("item_id", streamEvent.ItemID).Msg("Image generation in progress")

		case "response.image_generation_call.completed":
			touchTyping()
			// Image generation completed - the actual image data will be in response.completed
			tool, exists := activeTools[streamEvent.ItemID]
			callID := ""
			if exists && tool != nil {
				callID = tool.callID
			}
			if callID == "" {
				callID = streamEvent.ItemID
			}
			if exists {
				// Track tool call
				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        callID,
					ToolName:      "image_generation",
					ToolType:      string(tool.toolType),
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(ResultStatusSuccess),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
				})
			}
			log.Info().Str("item_id", streamEvent.ItemID).Msg("Image generation completed")

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_progress", map[string]any{
				"tool_name": "image_generation",
				"status":    "completed",
				"call_id":   callID,
				"progress": map[string]any{
					"message": "Image generation completed",
					"percent": 100,
				},
			}, rawPayload)

		case "response.image_generation_call.partial_image":
			touchTyping()
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "image_partial", map[string]any{
				"item_id":   streamEvent.ItemID,
				"index":     streamEvent.PartialImageIndex,
				"image_b64": streamEvent.PartialImageB64,
			}, rawPayload)

		case "response.output_text.annotation.added":
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "annotation", map[string]any{
				"annotation": streamEvent.Annotation,
				"index":      streamEvent.AnnotationIndex,
			}, rawPayload)

		case "response.completed":
			state.completedAtMs = time.Now().UnixMilli()

			if streamEvent.Response.Usage.TotalTokens > 0 || streamEvent.Response.Usage.InputTokens > 0 || streamEvent.Response.Usage.OutputTokens > 0 {
				state.promptTokens = streamEvent.Response.Usage.InputTokens
				state.completionTokens = streamEvent.Response.Usage.OutputTokens
				state.reasoningTokens = streamEvent.Response.Usage.OutputTokensDetails.ReasoningTokens
				state.totalTokens = streamEvent.Response.Usage.TotalTokens
			}

			if streamEvent.Response.Status == "completed" {
				state.finishReason = "stop"
			} else {
				state.finishReason = string(streamEvent.Response.Status)
			}
			// Capture response ID for persistence (will save to DB and portal after streaming completes)
			if streamEvent.Response.ID != "" {
				state.responseID = streamEvent.Response.ID
			}

			// Extract any generated images from response output
			for _, output := range streamEvent.Response.Output {
				if output.Type == "image_generation_call" {
					imgOutput := output.AsImageGenerationCall()
					if imgOutput.Status == "completed" && imgOutput.Result != "" {
						state.pendingImages = append(state.pendingImages, generatedImage{
							itemID:   imgOutput.ID,
							imageB64: imgOutput.Result,
							turnID:   state.turnID,
						})
						log.Debug().Str("item_id", imgOutput.ID).Msg("Captured generated image from response")
					}
				}
			}

			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "complete", map[string]any{
				"response_id":   state.responseID,
				"usage":         streamEvent.Response.Usage,
				"finish_reason": state.finishReason,
			}, rawPayload)

			// Emit final status
			oc.emitStatusEvent(ctx, portal, state, "finalizing", "Completing response...", nil)

			log.Debug().Str("reason", state.finishReason).Str("response_id", state.responseID).Int("images", len(state.pendingImages)).Msg("Response stream completed")

		case "error":
			apiErr := fmt.Errorf("API error: %s", streamEvent.Message)
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "error", map[string]any{
				"message": streamEvent.Message,
			}, rawPayload)
			logResponsesFailure(log, apiErr, params, meta, messages, "stream_event_error")
			// Check for context length error
			if strings.Contains(streamEvent.Message, "context_length") || strings.Contains(streamEvent.Message, "token") {
				return false, &ContextLengthError{
					OriginalError: fmt.Errorf("%s", streamEvent.Message),
				}, nil
			}
			if state.initialEventID != "" {
				return false, nil, &NonFallbackError{Err: apiErr}
			}
			return false, nil, &PreDeltaError{Err: apiErr}
		default:
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "raw_only", nil, rawPayload)
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logResponsesFailure(log, err, params, meta, messages, "stream_err")
		cle := ParseContextLengthError(err)
		if cle != nil {
			return false, cle, nil
		}
		if state.initialEventID != "" {
			return false, nil, &NonFallbackError{Err: err}
		}
		return false, nil, &PreDeltaError{Err: err}
	}

	// If there are pending function outputs, send them back to the API for continuation
	// This loop continues until the model generates a response without tool calls
	for len(state.pendingFunctionOutputs) > 0 && state.responseID != "" {
		log.Debug().
			Int("pending_outputs", len(state.pendingFunctionOutputs)).
			Str("previous_response_id", state.responseID).
			Msg("Continuing response with function call outputs")

		// Build continuation request with function call outputs
		continuationParams := oc.buildContinuationParams(state, meta)

		// OpenRouter Responses API is stateless; persist tool calls in base input.
		if oc.isOpenRouterProvider() && len(state.baseInput) > 0 {
			for _, output := range state.pendingFunctionOutputs {
				if output.name != "" {
					args := output.arguments
					if strings.TrimSpace(args) == "" {
						args = "{}"
					}
					state.baseInput = append(state.baseInput, responses.ResponseInputItemParamOfFunctionCall(args, output.callID, output.name))
				}
				state.baseInput = append(state.baseInput, buildFunctionCallOutputItem(output.callID, output.output, true))
			}
		}

		// Clear pending outputs (they're being sent)
		state.pendingFunctionOutputs = nil

		// Reset active tools for new iteration
		activeTools = make(map[string]*activeToolCall)

		// Start continuation stream
		stream = oc.api.Responses.NewStreaming(ctx, continuationParams)
		if stream == nil {
			initErr := fmt.Errorf("continuation streaming not available")
			logResponsesFailure(log, initErr, continuationParams, meta, messages, "continuation_init")
			break
		}

		// Process continuation stream events
		for stream.Next() {
			streamEvent := stream.Current()
			rawPayload := &StreamEventRawPayload{Type: streamEvent.Type, JSON: streamEvent.RawJSON()}

			switch streamEvent.Type {
			case "response.output_text.delta":
				touchTyping()
				state.accumulated.WriteString(streamEvent.Delta)

				if state.firstToken && state.accumulated.Len() > 0 {
					state.firstToken = false
					state.firstTokenAtMs = time.Now().UnixMilli()
					if !state.suppressSend {
						oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
						state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID, state.sourceEventID)
						if state.initialEventID == "" {
							log.Error().Msg("Failed to send initial streaming message (continuation)")
							return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message (continuation)")}
						}
						oc.emitStatusEvent(ctx, portal, state, "generating", "Generating response...", nil)
					}
				}
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "text_delta", map[string]any{
					"text": streamEvent.Delta,
				}, rawPayload)

			case "response.reasoning_text.delta":
				touchTyping()
				state.reasoning.WriteString(streamEvent.Delta)
				if state.firstToken && state.reasoning.Len() > 0 {
					state.firstToken = false
					state.firstTokenAtMs = time.Now().UnixMilli()
					if !state.suppressSend {
						oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
						state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
						if state.initialEventID == "" {
							log.Error().Msg("Failed to send initial streaming message (continuation)")
							return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message (continuation)")}
						}
						oc.emitStatusEvent(ctx, portal, state, "thinking", "Thinking...", nil)
					}
				}
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "reasoning_delta", map[string]any{
					"text": streamEvent.Delta,
				}, rawPayload)

			case "response.refusal.delta":
				touchTyping()
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "refusal_delta", map[string]any{
					"text": streamEvent.Delta,
				}, rawPayload)

			case "response.function_call_arguments.delta":
				touchTyping()
				tool, exists := activeTools[streamEvent.ItemID]
				if !exists {
					callID := streamEvent.ItemID
					if strings.TrimSpace(callID) == "" {
						callID = NewCallID()
					}
					tool = &activeToolCall{
						callID:      callID,
						toolName:    streamEvent.Name,
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					activeTools[streamEvent.ItemID] = tool
					if state.initialEventID == "" && !state.suppressSend {
						oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
					}
					oc.emitStatusEvent(ctx, portal, state, "tool_use", fmt.Sprintf("Calling %s...", streamEvent.Name), &GenerationDetails{
						CurrentTool: streamEvent.Name,
						CallID:      tool.callID,
					})
				}
				tool.input.WriteString(streamEvent.Delta)
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_call_delta", map[string]any{
					"item_id":         streamEvent.ItemID,
					"name":            streamEvent.Name,
					"arguments_delta": streamEvent.Delta,
					"call_id":         tool.callID,
				}, rawPayload)

			case "response.function_call_arguments.done":
				touchTyping()
				tool, exists := activeTools[streamEvent.ItemID]
				if !exists {
					callID := streamEvent.ItemID
					if strings.TrimSpace(callID) == "" {
						callID = NewCallID()
					}
					tool = &activeToolCall{
						callID:      callID,
						toolName:    streamEvent.Name,
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					tool.input.WriteString(streamEvent.Arguments)
					activeTools[streamEvent.ItemID] = tool
				}

				tool.itemID = streamEvent.ItemID

				toolName := strings.TrimSpace(tool.toolName)
				if toolName == "" {
					toolName = strings.TrimSpace(streamEvent.Name)
				}
				tool.toolName = toolName
				argsJSON := strings.TrimSpace(tool.input.String())
				if argsJSON == "" {
					argsJSON = strings.TrimSpace(streamEvent.Arguments)
				}
				argsJSON = normalizeToolArgsJSON(argsJSON)

				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "tool_call_done", map[string]any{
					"item_id":   streamEvent.ItemID,
					"name":      toolName,
					"arguments": argsJSON,
					"call_id":   tool.callID,
				}, rawPayload)

				resultStatus := ResultStatusSuccess
				var result string
				if !oc.isToolEnabled(meta, toolName) {
					resultStatus = ResultStatusError
					result = fmt.Sprintf("Error: tool %s is disabled", toolName)
				} else {
					toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
						Client:        oc,
						Portal:        portal,
						Meta:          meta,
						SourceEventID: state.sourceEventID,
					})
					var err error
					result, err = oc.executeBuiltinTool(toolCtx, portal, toolName, argsJSON)
					if err != nil {
						log.Warn().Err(err).Str("tool", toolName).Msg("Tool execution failed (continuation)")
						result = fmt.Sprintf("Error: %s", err.Error())
						resultStatus = ResultStatusError
					}
				}

				// Check for TTS audio result (AUDIO: prefix)
				displayResult := result
				if strings.HasPrefix(result, TTSResultPrefix) {
					audioB64 := strings.TrimPrefix(result, TTSResultPrefix)
					audioData, err := base64.StdEncoding.DecodeString(audioB64)
					if err != nil {
						log.Warn().Err(err).Msg("Failed to decode TTS audio (continuation)")
						displayResult = "Error: failed to decode TTS audio"
						resultStatus = ResultStatusError
					} else {
						if _, err := oc.sendGeneratedAudio(ctx, portal, audioData, "audio/mpeg", state.turnID); err != nil {
							log.Warn().Err(err).Msg("Failed to send TTS audio (continuation)")
							displayResult = "Error: failed to send TTS audio"
							resultStatus = ResultStatusError
						} else {
							displayResult = "Audio message sent successfully"
						}
					}
					result = displayResult
				}

				// Check for image generation result (IMAGE: / IMAGES: prefix)
				if strings.HasPrefix(result, ImagesResultPrefix) {
					payload := strings.TrimPrefix(result, ImagesResultPrefix)
					var images []string
					if err := json.Unmarshal([]byte(payload), &images); err != nil {
						log.Warn().Err(err).Msg("Failed to parse generated images payload (continuation)")
						displayResult = "Error: failed to parse generated images"
						resultStatus = ResultStatusError
					} else {
						success := 0
						for _, imageB64 := range images {
							imageData, mimeType, err := decodeBase64Image(imageB64)
							if err != nil {
								log.Warn().Err(err).Msg("Failed to decode generated image (continuation)")
								continue
							}
							if _, err := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, state.turnID); err != nil {
								log.Warn().Err(err).Msg("Failed to send generated image (continuation)")
								continue
							}
							success++
						}
						if success == len(images) && success > 0 {
							displayResult = fmt.Sprintf("Images generated and sent successfully (%d)", success)
						} else if success > 0 {
							displayResult = fmt.Sprintf("Images generated with %d/%d sent successfully", success, len(images))
							resultStatus = ResultStatusError
						} else {
							displayResult = "Error: failed to send generated images"
							resultStatus = ResultStatusError
						}
					}
					result = displayResult
				} else if strings.HasPrefix(result, ImageResultPrefix) {
					imageB64 := strings.TrimPrefix(result, ImageResultPrefix)
					imageData, mimeType, err := decodeBase64Image(imageB64)
					if err != nil {
						log.Warn().Err(err).Msg("Failed to decode generated image (continuation)")
						displayResult = "Error: failed to decode generated image"
						resultStatus = ResultStatusError
					} else {
						if _, err := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, state.turnID); err != nil {
							log.Warn().Err(err).Msg("Failed to send generated image (continuation)")
							displayResult = "Error: failed to send generated image"
							resultStatus = ResultStatusError
						} else {
							displayResult = "Image generated and sent successfully"
						}
					}
					result = displayResult
				}

				tool.result = result
				state.pendingFunctionOutputs = append(state.pendingFunctionOutputs, functionCallOutput{
					callID:    streamEvent.ItemID,
					name:      toolName,
					arguments: argsJSON,
					output:    result,
				})

				var inputMap map[string]any
				_ = json.Unmarshal([]byte(argsJSON), &inputMap)

				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        tool.callID,
					ToolName:      toolName,
					ToolType:      string(tool.toolType),
					Input:         inputMap,
					Output:        map[string]any{"result": result},
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(resultStatus),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
				})

				toolStatus := ToolStatusCompleted
				if resultStatus != ResultStatusSuccess {
					toolStatus = ToolStatusFailed
				}
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceInternal, "tool_result", map[string]any{
					"call_id":       tool.callID,
					"tool_name":     toolName,
					"result":        result,
					"status":        string(toolStatus),
					"result_status": string(resultStatus),
				}, nil)
				oc.emitStatusEvent(ctx, portal, state, "generating", "Continuing generation...", nil)

			case "response.completed":
				state.completedAtMs = time.Now().UnixMilli()
				if streamEvent.Response.Usage.TotalTokens > 0 || streamEvent.Response.Usage.InputTokens > 0 || streamEvent.Response.Usage.OutputTokens > 0 {
					state.promptTokens = streamEvent.Response.Usage.InputTokens
					state.completionTokens = streamEvent.Response.Usage.OutputTokens
					state.reasoningTokens = streamEvent.Response.Usage.OutputTokensDetails.ReasoningTokens
					state.totalTokens = streamEvent.Response.Usage.TotalTokens
				}
				if streamEvent.Response.Status == "completed" {
					state.finishReason = "stop"
				} else {
					state.finishReason = string(streamEvent.Response.Status)
				}
				if streamEvent.Response.ID != "" {
					state.responseID = streamEvent.Response.ID
				}
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "complete", map[string]any{
					"response_id":   state.responseID,
					"usage":         streamEvent.Response.Usage,
					"finish_reason": state.finishReason,
				}, rawPayload)
				log.Debug().Str("reason", state.finishReason).Str("response_id", state.responseID).Msg("Continuation stream completed")

			case "error":
				apiErr := fmt.Errorf("API error: %s", streamEvent.Message)
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "error", map[string]any{
					"message": streamEvent.Message,
				}, rawPayload)
				logResponsesFailure(log, apiErr, continuationParams, meta, messages, "continuation_event_error")
			default:
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceResponses, "raw_only", nil, rawPayload)
			}
		}

		if err := stream.Err(); err != nil {
			logResponsesFailure(log, err, continuationParams, meta, messages, "continuation_err")
			break
		}
	}

	if typingCtrl != nil {
		typingCtrl.MarkRunComplete()
	}

	// Send final message to persist complete content with metadata (including reasoning)
	if state.initialEventID != "" || state.heartbeat != nil {
		oc.sendFinalAssistantTurn(ctx, portal, state, meta)
		if state.initialEventID != "" && !state.suppressSave {
			oc.saveAssistantMessage(ctx, log, portal, state, meta)
		}
	}

	// Send any generated images as separate messages
	for _, img := range state.pendingImages {
		imageData, mimeType, err := decodeBase64Image(img.imageB64)
		if err != nil {
			log.Warn().Err(err).Str("item_id", img.itemID).Msg("Failed to decode generated image")
			continue
		}
		eventID, err := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, img.turnID)
		if err != nil {
			log.Warn().Err(err).Str("item_id", img.itemID).Msg("Failed to send generated image to Matrix")
			continue
		}
		log.Info().Stringer("event_id", eventID).Str("item_id", img.itemID).Msg("Sent generated image to Matrix")
	}

	log.Info().
		Str("turn_id", state.turnID).
		Str("finish_reason", state.finishReason).
		Int("content_length", state.accumulated.Len()).
		Int("reasoning_length", state.reasoning.Len()).
		Int("tool_calls", len(state.toolCalls)).
		Str("response_id", state.responseID).
		Int("images_sent", len(state.pendingImages)).
		Msg("Responses API streaming finished")

	// Generate room title after first response
	oc.maybeGenerateTitle(ctx, portal, state.accumulated.String())

	return true, nil, nil
}

// buildContinuationParams builds params for continuing a response after tool execution
func (oc *AIClient) buildContinuationParams(state *streamingState, meta *PortalMetadata) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(oc.effectiveModelForAPI(meta)),
		MaxOutputTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	if systemPrompt := oc.effectivePrompt(meta); systemPrompt != "" {
		params.Instructions = openai.String(systemPrompt)
	}

	isOpenRouter := oc.isOpenRouterProvider()
	if !isOpenRouter {
		params.PreviousResponseID = openai.String(state.responseID)
	}

	// Build function call outputs as input
	var input responses.ResponseInputParam
	if isOpenRouter && len(state.baseInput) > 0 {
		// OpenRouter Responses API is stateless: include full history plus tool calls.
		input = append(input, state.baseInput...)
	}
	for _, output := range state.pendingFunctionOutputs {
		if isOpenRouter && output.name != "" {
			args := output.arguments
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			input = append(input, responses.ResponseInputItemParamOfFunctionCall(args, output.callID, output.name))
		}
		input = append(input, buildFunctionCallOutputItem(output.callID, output.output, isOpenRouter))
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

	// OpenRouter's Responses API only supports function-type tools.
	if meta.Capabilities.SupportsToolCalling {
		enabledTools := GetEnabledBuiltinTools(func(name string) bool {
			return oc.isToolEnabled(meta, name)
		})
		if len(enabledTools) > 0 {
			strictMode := resolveToolStrictMode(oc.isOpenRouterProvider())
			params.Tools = append(params.Tools, ToOpenAITools(enabledTools, strictMode, &oc.log)...)
		}
	}

	// Add boss tools for Boss agent rooms (needed for multi-turn tool use)
	agentID := resolveAgentID(meta)
	if hasBossAgent(meta) || agents.IsBossAgent(agentID) {
		var enabledBoss []*tools.Tool
		for _, tool := range tools.BossTools() {
			if oc.isToolEnabled(meta, tool.Name) {
				enabledBoss = append(enabledBoss, tool)
			}
		}
		strictMode := resolveToolStrictMode(oc.isOpenRouterProvider())
		params.Tools = append(params.Tools, bossToolsToOpenAI(enabledBoss, strictMode, &oc.log)...)
	}

	// Add session tools for non-boss rooms (needed for multi-turn tool use)
	if meta.Capabilities.SupportsToolCalling && !(hasBossAgent(meta) || agents.IsBossAgent(agentID)) {
		var enabledSessions []*tools.Tool
		for _, tool := range tools.SessionTools() {
			if oc.isToolEnabled(meta, tool.Name) {
				enabledSessions = append(enabledSessions, tool)
			}
		}
		if len(enabledSessions) > 0 {
			strictMode := resolveToolStrictMode(oc.isOpenRouterProvider())
			params.Tools = append(params.Tools, bossToolsToOpenAI(enabledSessions, strictMode, &oc.log)...)
		}
	}

	if oc.isOpenRouterProvider() {
		params.Tools = renameWebSearchToolParams(params.Tools)
	}

	// Prevent duplicate tool names (Anthropic rejects duplicates)
	params.Tools = dedupeToolParams(params.Tools)
	logToolParamDuplicates(&oc.log, params.Tools)

	return params
}

// streamChatCompletions handles streaming using Chat Completions API (for audio support)
// This is used as a fallback when the prompt contains audio content, since
// SDK v3.16.0 has ResponseInputAudioParam defined but NOT wired into ResponseInputContentUnionParam.
func (oc *AIClient) streamChatCompletions(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	messages []openai.ChatCompletionMessageParamUnion,
) (bool, *ContextLengthError, error) {
	log := zerolog.Ctx(ctx).With().
		Str("action", "stream_chat_completions").
		Str("portal", string(portal.ID)).
		Logger()

	// Initialize streaming state with source event ID for [[reply_to_current]] support
	var sourceEventID id.EventID
	if evt != nil {
		sourceEventID = evt.ID
	}
	state := newStreamingState(ctx, meta, sourceEventID)

	// Ensure model ghost is in the room before any operations
	if !state.suppressSend {
		if err := oc.ensureModelInRoom(ctx, portal); err != nil {
			log.Warn().Err(err).Msg("Failed to ensure model is in room")
			// Continue anyway - typing will fail gracefully
		}
	}

	// Create typing controller with TTL and automatic refresh
	var typingCtrl *TypingController
	touchTyping := func() {}
	if !state.suppressSend && state.heartbeat == nil {
		typingCtrl = NewTypingController(oc, ctx, portal)
		typingCtrl.Start()
		defer typingCtrl.Stop()
		touchTyping = func() {
			typingCtrl.RefreshTTL()
		}
	}

	// Apply proactive context pruning if enabled
	messages = oc.applyProactivePruning(ctx, messages, meta)

	params := openai.ChatCompletionNewParams{
		Model:    oc.effectiveModelForAPI(meta),
		Messages: messages,
	}
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: param.NewOpt(true),
	}
	if maxTokens := oc.effectiveMaxTokens(meta); maxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	}
	if temp := oc.effectiveTemperature(meta); temp > 0 {
		params.Temperature = openai.Float(temp)
	}
	if meta.Capabilities.SupportsToolCalling {
		enabledTools := GetEnabledBuiltinTools(func(name string) bool {
			return oc.isToolEnabled(meta, name)
		})
		if len(enabledTools) > 0 {
			params.Tools = append(params.Tools, ToOpenAIChatTools(enabledTools, &oc.log)...)
		}
		if !oc.isBuilderRoom(portal) {
			var enabledSessions []*tools.Tool
			for _, tool := range tools.SessionTools() {
				if oc.isToolEnabled(meta, tool.Name) {
					enabledSessions = append(enabledSessions, tool)
				}
			}
			if len(enabledSessions) > 0 {
				params.Tools = append(params.Tools, bossToolsToChatTools(enabledSessions, &oc.log)...)
			}
		}
		if hasBossAgent(meta) || oc.isBuilderRoom(portal) {
			var enabledBoss []*tools.Tool
			for _, tool := range tools.BossTools() {
				if oc.isToolEnabled(meta, tool.Name) {
					enabledBoss = append(enabledBoss, tool)
				}
			}
			params.Tools = append(params.Tools, bossToolsToChatTools(enabledBoss, &oc.log)...)
		}
		params.Tools = dedupeChatToolParams(params.Tools)
	}

	stream := oc.api.Chat.Completions.NewStreaming(ctx, params)
	if stream == nil {
		initErr := fmt.Errorf("chat completions streaming not available")
		logChatCompletionsFailure(log, initErr, params, meta, messages, "stream_init")
		return false, nil, &PreDeltaError{Err: initErr}
	}

	// Track active tool calls by index
	activeTools := make(map[int]*activeToolCall)

	oc.emitStatusEvent(ctx, portal, state, "starting", "Starting generation...", nil)

	for stream.Next() {
		chunk := stream.Current()
		rawPayload := &StreamEventRawPayload{Type: string(chunk.Object), JSON: chunk.RawJSON()}
		emitted := false

		if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			state.promptTokens = chunk.Usage.PromptTokens
			state.completionTokens = chunk.Usage.CompletionTokens
			state.reasoningTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
			state.totalTokens = chunk.Usage.TotalTokens
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				touchTyping()
				state.accumulated.WriteString(choice.Delta.Content)

				if state.firstToken && state.accumulated.Len() > 0 {
					state.firstToken = false
					state.firstTokenAtMs = time.Now().UnixMilli()
					if !state.suppressSend {
						oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
						state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID, state.sourceEventID)
						if state.initialEventID == "" {
							log.Error().Msg("Failed to send initial streaming message")
							return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message")}
						}
						oc.emitStatusEvent(ctx, portal, state, "generating", "Generating response...", nil)
					}
				}
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceChatCompletions, "text_delta", map[string]any{
					"text": choice.Delta.Content,
				}, rawPayload)
				emitted = true
			}

			if choice.Delta.Refusal != "" {
				touchTyping()
				oc.emitStreamEvent(ctx, portal, state, StreamEventSourceChatCompletions, "refusal_delta", map[string]any{
					"text": choice.Delta.Refusal,
				}, rawPayload)
				emitted = true
			}

			// Handle tool calls from Chat Completions API
			for _, toolDelta := range choice.Delta.ToolCalls {
				touchTyping()
				toolIdx := int(toolDelta.Index)
				tool, exists := activeTools[toolIdx]
				if !exists {
					callID := toolDelta.ID
					if strings.TrimSpace(callID) == "" {
						callID = NewCallID()
					}
					tool = &activeToolCall{
						callID:      callID,
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					activeTools[toolIdx] = tool
				}

				// Capture tool ID if provided (used by OpenAI for tracking)
				if toolDelta.ID != "" && tool.callID == "" {
					tool.callID = toolDelta.ID
				}

				// Update tool name if provided in this delta
				if toolDelta.Function.Name != "" {
					wasUnnamed := tool.toolName == ""
					tool.toolName = toolDelta.Function.Name

					if wasUnnamed {
						oc.emitStatusEvent(ctx, portal, state, "tool_use",
							fmt.Sprintf("Calling %s...", tool.toolName), &GenerationDetails{
								CurrentTool: tool.toolName,
								CallID:      tool.callID,
							})
					}
				}

				// Accumulate arguments
				if toolDelta.Function.Arguments != "" {
					tool.input.WriteString(toolDelta.Function.Arguments)
					oc.emitStreamEvent(ctx, portal, state, StreamEventSourceChatCompletions, "tool_call_delta", map[string]any{
						"index":           toolIdx,
						"id":              toolDelta.ID,
						"name":            tool.toolName,
						"arguments_delta": toolDelta.Function.Arguments,
						"call_id":         tool.callID,
					}, rawPayload)
					emitted = true
				}
			}

			if choice.FinishReason != "" {
				state.finishReason = string(choice.FinishReason)
				if choice.FinishReason == "tool_calls" {
					calls := make([]map[string]any, 0, len(activeTools))
					for idx, tool := range activeTools {
						args := strings.TrimSpace(tool.input.String())
						if args == "" {
							args = "{}"
						}
						calls = append(calls, map[string]any{
							"index":     idx,
							"id":        tool.callID,
							"call_id":   tool.callID,
							"name":      tool.toolName,
							"arguments": args,
						})
					}
					oc.emitStreamEvent(ctx, portal, state, StreamEventSourceChatCompletions, "tool_call_done", map[string]any{
						"calls": calls,
					}, rawPayload)
					emitted = true
				}
			}
		}

		if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceChatCompletions, "complete", map[string]any{
				"usage":         chunk.Usage,
				"finish_reason": state.finishReason,
			}, rawPayload)
			emitted = true
		}

		if !emitted {
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceChatCompletions, "raw_only", nil, rawPayload)
		}
	}

	if err := stream.Err(); err != nil {
		if cle := ParseContextLengthError(err); cle != nil {
			return false, cle, nil
		}
		logChatCompletionsFailure(log, err, params, meta, messages, "stream_err")
		if state.initialEventID != "" {
			return false, nil, &NonFallbackError{Err: err}
		}
		return false, nil, &PreDeltaError{Err: err}
	}

	// Execute any accumulated tool calls
	for _, tool := range activeTools {
		if tool.toolName != "" && oc.isToolEnabled(meta, tool.toolName) {
			touchTyping()
			// Wrap context with bridge info for tools that need it (e.g., channel-edit, react)
			toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
				Client:        oc,
				Portal:        portal,
				Meta:          meta,
				SourceEventID: state.sourceEventID,
			})
			argsJSON := normalizeToolArgsJSON(tool.input.String())
			result, err := oc.executeBuiltinTool(toolCtx, portal, tool.toolName, argsJSON)
			resultStatus := ResultStatusSuccess
			if err != nil {
				log.Warn().Err(err).Str("tool", tool.toolName).Msg("Tool execution failed (Chat Completions)")
				result = fmt.Sprintf("Error: %s", err.Error())
				resultStatus = ResultStatusError
			}

			// Check for TTS audio result (AUDIO: prefix)
			if strings.HasPrefix(result, TTSResultPrefix) {
				audioB64 := strings.TrimPrefix(result, TTSResultPrefix)
				audioData, decodeErr := base64.StdEncoding.DecodeString(audioB64)
				if decodeErr != nil {
					log.Warn().Err(decodeErr).Msg("Failed to decode TTS audio (Chat Completions)")
					result = "Error: failed to decode TTS audio"
					resultStatus = ResultStatusError
				} else {
					if _, sendErr := oc.sendGeneratedAudio(ctx, portal, audioData, "audio/mpeg", state.turnID); sendErr != nil {
						log.Warn().Err(sendErr).Msg("Failed to send TTS audio (Chat Completions)")
						result = "Error: failed to send TTS audio"
						resultStatus = ResultStatusError
					} else {
						result = "Audio message sent successfully"
					}
				}
			}

			// Check for image generation result (IMAGE: / IMAGES: prefix)
			if strings.HasPrefix(result, ImagesResultPrefix) {
				payload := strings.TrimPrefix(result, ImagesResultPrefix)
				var images []string
				if err := json.Unmarshal([]byte(payload), &images); err != nil {
					log.Warn().Err(err).Msg("Failed to parse generated images payload (Chat Completions)")
					result = "Error: failed to parse generated images"
					resultStatus = ResultStatusError
				} else {
					success := 0
					for _, imageB64 := range images {
						imageData, mimeType, decodeErr := decodeBase64Image(imageB64)
						if decodeErr != nil {
							log.Warn().Err(decodeErr).Msg("Failed to decode generated image (Chat Completions)")
							continue
						}
						if _, sendErr := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, state.turnID); sendErr != nil {
							log.Warn().Err(sendErr).Msg("Failed to send generated image (Chat Completions)")
							continue
						}
						success++
					}
					if success == len(images) && success > 0 {
						result = fmt.Sprintf("Images generated and sent successfully (%d)", success)
					} else if success > 0 {
						result = fmt.Sprintf("Images generated with %d/%d sent successfully", success, len(images))
						resultStatus = ResultStatusError
					} else {
						result = "Error: failed to send generated images"
						resultStatus = ResultStatusError
					}
				}
			} else if strings.HasPrefix(result, ImageResultPrefix) {
				imageB64 := strings.TrimPrefix(result, ImageResultPrefix)
				imageData, mimeType, decodeErr := decodeBase64Image(imageB64)
				if decodeErr != nil {
					log.Warn().Err(decodeErr).Msg("Failed to decode generated image (Chat Completions)")
					result = "Error: failed to decode generated image"
					resultStatus = ResultStatusError
				} else {
					if _, sendErr := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, state.turnID); sendErr != nil {
						log.Warn().Err(sendErr).Msg("Failed to send generated image (Chat Completions)")
						result = "Error: failed to send generated image"
						resultStatus = ResultStatusError
					} else {
						result = "Image generated and sent successfully"
					}
				}
			}

			// Parse input for storage
			var inputMap map[string]any
			_ = json.Unmarshal([]byte(argsJSON), &inputMap)

			// Track tool call in metadata
			state.toolCalls = append(state.toolCalls, ToolCallMetadata{
				CallID:        tool.callID,
				ToolName:      tool.toolName,
				ToolType:      string(tool.toolType),
				Input:         inputMap,
				Output:        map[string]any{"result": result},
				Status:        string(ToolStatusCompleted),
				ResultStatus:  string(resultStatus),
				StartedAtMs:   tool.startedAtMs,
				CompletedAtMs: time.Now().UnixMilli(),
			})

			toolStatus := ToolStatusCompleted
			if resultStatus != ResultStatusSuccess {
				toolStatus = ToolStatusFailed
			}
			oc.emitStreamEvent(ctx, portal, state, StreamEventSourceInternal, "tool_result", map[string]any{
				"call_id":       tool.callID,
				"tool_name":     tool.toolName,
				"result":        result,
				"status":        string(toolStatus),
				"result_status": string(resultStatus),
			}, nil)

			// Update status back to generating if there might be more content
			oc.emitStatusEvent(ctx, portal, state, "generating", "Continuing generation...", nil)
		}
	}

	if typingCtrl != nil {
		typingCtrl.MarkRunComplete()
	}

	state.completedAtMs = time.Now().UnixMilli()
	oc.emitStatusEvent(ctx, portal, state, "completed", "Generation complete", nil)

	// Send final edit and save to database
	if state.initialEventID != "" {
		oc.sendFinalAssistantTurn(ctx, portal, state, meta)
		if !state.suppressSave {
			oc.saveAssistantMessage(ctx, log, portal, state, meta)
		}
	}

	log.Info().
		Str("turn_id", state.turnID).
		Str("finish_reason", state.finishReason).
		Int("content_length", state.accumulated.Len()).
		Int("tool_calls", len(state.toolCalls)).
		Msg("Chat Completions streaming finished")

	oc.maybeGenerateTitle(ctx, portal, state.accumulated.String())
	return true, nil, nil
}

// convertToResponsesInput converts Chat Completion messages to Responses API input items
// Supports native multimodal content: images (ResponseInputImageParam), files/PDFs (ResponseInputFileParam)
// Note: Audio is handled via Chat Completions API fallback (SDK v3.16.0 lacks Responses API audio union support)
func (oc *AIClient) convertToResponsesInput(messages []openai.ChatCompletionMessageParamUnion, _ *PortalMetadata) responses.ResponseInputParam {
	var input responses.ResponseInputParam

	for _, msg := range messages {
		if msg.OfUser != nil {
			var contentParts responses.ResponseInputMessageContentListParam
			hasMultimodal := false
			textContent := ""

			if msg.OfUser.Content.OfString.Value != "" {
				textContent = msg.OfUser.Content.OfString.Value
			}

			if len(msg.OfUser.Content.OfArrayOfContentParts) > 0 {
				for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
					if part.OfText != nil && part.OfText.Text != "" {
						if textContent != "" {
							textContent += "\n"
						}
						textContent += part.OfText.Text
					}
					if part.OfImageURL != nil && part.OfImageURL.ImageURL.URL != "" {
						hasMultimodal = true
						detail := responses.ResponseInputImageDetailAuto
						switch part.OfImageURL.ImageURL.Detail {
						case "low":
							detail = responses.ResponseInputImageDetailLow
						case "high":
							detail = responses.ResponseInputImageDetailHigh
						}
						contentParts = append(contentParts, responses.ResponseInputContentUnionParam{
							OfInputImage: &responses.ResponseInputImageParam{
								ImageURL: openai.String(part.OfImageURL.ImageURL.URL),
								Detail:   detail,
							},
						})
					}
					if part.OfFile != nil && part.OfFile.File.FileData.Value != "" {
						hasMultimodal = true
						contentParts = append(contentParts, responses.ResponseInputContentUnionParam{
							OfInputFile: &responses.ResponseInputFileParam{
								FileData: openai.String(part.OfFile.File.FileData.Value),
							},
						})
					}
					// Note: Audio handled by Chat Completions fallback, skip here
				}
			}

			if textContent != "" {
				textPart := responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{
						Text: textContent,
					},
				}
				contentParts = append([]responses.ResponseInputContentUnionParam{textPart}, contentParts...)
			}

			if hasMultimodal && len(contentParts) > 0 {
				input = append(input, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfInputItemContentList: contentParts,
						},
					},
				})
			} else if textContent != "" {
				input = append(input, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: openai.String(textContent),
						},
					},
				})
			}
			continue
		}

		content, role := extractMessageContent(msg)
		if role == "" || content == "" {
			continue
		}

		var responsesRole responses.EasyInputMessageRole
		switch role {
		case "system":
			responsesRole = responses.EasyInputMessageRoleSystem
		case "assistant":
			responsesRole = responses.EasyInputMessageRoleAssistant
		default:
			responsesRole = responses.EasyInputMessageRoleUser
		}

		input = append(input, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responsesRole,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(content),
				},
			},
		})
	}

	return input
}

// hasAudioContent checks if the prompt contains audio content
func hasAudioContent(messages []openai.ChatCompletionMessageParamUnion) bool {
	for _, msg := range messages {
		if msg.OfUser != nil && len(msg.OfUser.Content.OfArrayOfContentParts) > 0 {
			for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
				if part.OfInputAudio != nil {
					return true
				}
			}
		}
	}
	return false
}

// hasMultimodalContent checks if the prompt contains non-text content (image, file, audio).
func hasMultimodalContent(messages []openai.ChatCompletionMessageParamUnion) bool {
	for _, msg := range messages {
		if msg.OfUser != nil && len(msg.OfUser.Content.OfArrayOfContentParts) > 0 {
			for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
				if part.OfImageURL != nil || part.OfFile != nil || part.OfInputAudio != nil {
					return true
				}
			}
		}
	}
	return false
}
