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
	skipNextTextDelta      bool

	// Directive processing
	sourceEventID id.EventID // The triggering user message event ID (for [[reply_to_current]])
}

// newStreamingState creates a new streaming state with initialized fields
func newStreamingState(meta *PortalMetadata, sourceEventID id.EventID) *streamingState {
	return &streamingState{
		turnID:        NewTurnID(),
		agentID:       meta.DefaultAgentID,
		startedAtMs:   time.Now().UnixMilli(),
		firstToken:    true,
		sourceEventID: sourceEventID,
	}
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

	// Ensure model ghost is in the room before any operations
	if err := oc.ensureModelInRoom(ctx, portal); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure model is in room")
		// Continue anyway - typing will fail gracefully
	}

	// Create typing controller with TTL and automatic refresh
	typingCtrl := NewTypingController(oc, ctx, portal)
	typingCtrl.Start()
	defer typingCtrl.Stop()
	touchTyping := func() {
		typingCtrl.RefreshTTL()
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
		log.Error().Msg("Failed to create Responses API streaming request")
		return false, nil, &PreDeltaError{Err: fmt.Errorf("responses streaming not available")}
	}

	// Initialize streaming state with turn tracking
	// Pass source event ID for [[reply_to_current]] directive support
	var sourceEventID id.EventID
	if evt != nil {
		sourceEventID = evt.ID
	}
	state := newStreamingState(meta, sourceEventID)

	// Store base input for OpenRouter stateless continuations
	if params.Input.OfInputItemList != nil {
		state.baseInput = params.Input.OfInputItemList
	}

	ensureInitialEvent := func() bool {
		if state.initialEventID != "" {
			return true
		}
		state.firstToken = false
		state.firstTokenAtMs = time.Now().UnixMilli()
		oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
		state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
		if state.initialEventID == "" {
			log.Error().Msg("Failed to send initial streaming message for tool call")
			return false
		}
		return true
	}

	// Track active tool calls
	activeTools := make(map[string]*activeToolCall)

	// Emit generation status: starting
	oc.emitGenerationStatus(ctx, portal, state, "starting", "Starting generation...", nil)

	// Process stream events - no debouncing, stream every delta immediately
	for stream.Next() {
		streamEvent := stream.Current()

		switch streamEvent.Type {
		case "response.output_text.delta":
			touchTyping()
			state.accumulated.WriteString(streamEvent.Delta)

			// First token - send initial message synchronously to capture event_id
			if state.firstToken && state.accumulated.Len() > 0 {
				state.firstToken = false
				state.firstTokenAtMs = time.Now().UnixMilli()
				// Ensure ghost display name is set before sending the first message
				oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
				state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID, state.sourceEventID)
				if state.initialEventID == "" {
					log.Error().Msg("Failed to send initial streaming message")
					return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message")}
				}
				// Initial message already includes this delta
				state.skipNextTextDelta = true
				// Update status to generating
				oc.emitGenerationStatus(ctx, portal, state, "generating", "Generating response...", nil)
			}

			// Stream every text delta immediately (no debouncing for snappy UX)
			if !state.firstToken && state.initialEventID != "" {
				if state.skipNextTextDelta {
					state.skipNextTextDelta = false
					break
				}
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentText, streamEvent.Delta, nil)
			}

		case "response.reasoning_text.delta":
			touchTyping()
			state.reasoning.WriteString(streamEvent.Delta)

			// Check if this is first content (reasoning before text)
			if state.firstToken && state.reasoning.Len() > 0 {
				state.firstToken = false
				state.firstTokenAtMs = time.Now().UnixMilli()
				oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
				// Send empty initial message - will be replaced with content later
				state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
				if state.initialEventID == "" {
					log.Error().Msg("Failed to send initial streaming message")
					return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message")}
				}
				// Update status to thinking
				oc.emitGenerationStatus(ctx, portal, state, "thinking", "Thinking...", nil)
			}

			// Stream reasoning tokens in real-time
			if !state.firstToken && state.initialEventID != "" {
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentReasoning, streamEvent.Delta, nil)
			}

		case "response.function_call_arguments.delta":
			touchTyping()
			// Get or create active tool call
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				if !ensureInitialEvent() {
					return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message for tool call")}
				}
				tool = &activeToolCall{
					callID:      NewCallID(),
					toolName:    streamEvent.Name,
					toolType:    ToolTypeFunction,
					startedAtMs: time.Now().UnixMilli(),
				}
				activeTools[streamEvent.ItemID] = tool

				// Send tool call timeline event
				tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)

				// Update status to tool_use
				oc.emitGenerationStatus(ctx, portal, state, "tool_use", fmt.Sprintf("Calling %s...", streamEvent.Name), &GenerationDetails{
					CurrentTool: streamEvent.Name,
					CallID:      tool.callID,
				})
			}

			// Accumulate arguments
			tool.input.WriteString(streamEvent.Delta)

			// Stream function call arguments as they arrive
			if state.initialEventID != "" {
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolInput, streamEvent.Delta, map[string]any{
					"call_id":   tool.callID,
					"tool_name": streamEvent.Name,
				})
			}

		case "response.function_call_arguments.done":
			touchTyping()
			// Function call complete - execute the tool and send result
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				// Create tool if we missed the delta events
				if !ensureInitialEvent() {
					return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message for tool call")}
				}
				tool = &activeToolCall{
					callID:      NewCallID(),
					toolName:    streamEvent.Name,
					toolType:    ToolTypeFunction,
					startedAtMs: time.Now().UnixMilli(),
				}
				tool.input.WriteString(streamEvent.Arguments)
				activeTools[streamEvent.ItemID] = tool
			}

			// Update tool name from done event (delta events don't have the name)
			if tool.toolName == "" && streamEvent.Name != "" {
				tool.toolName = streamEvent.Name
			}

			// Store the item ID for continuation (this is the call_id for the Responses API)
			tool.itemID = streamEvent.ItemID

			if !ensureInitialEvent() {
				return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message for tool call")}
			}

			toolName := strings.TrimSpace(tool.toolName)
			if toolName == "" {
				toolName = strings.TrimSpace(streamEvent.Name)
			}
			argsJSON := strings.TrimSpace(tool.input.String())
			if argsJSON == "" {
				argsJSON = strings.TrimSpace(streamEvent.Arguments)
			}
			argsJSON = normalizeToolArgsJSON(argsJSON)

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
			name := toolName
			state.pendingFunctionOutputs = append(state.pendingFunctionOutputs, functionCallOutput{
				callID:    streamEvent.ItemID,
				name:      name,
				arguments: args,
				output:    result,
			})

			// Parse input for storage
			var inputMap map[string]any
			_ = json.Unmarshal([]byte(argsJSON), &inputMap)

			// Send tool result timeline event
			resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, result, resultStatus)

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
				CallEventID:   tool.eventID.String(),
				ResultEventID: resultEventID.String(),
			})

			// Stream result
			state.sequenceNum++
			oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, result, map[string]any{
				"call_id":   tool.callID,
				"tool_name": streamEvent.Name,
				"status":    "completed",
			})

			// Update status back to generating
			oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)

		case "response.web_search_call.searching":
			touchTyping()
			// Web search starting
			tool := &activeToolCall{
				callID:      NewCallID(),
				toolName:    "web_search",
				toolType:    ToolTypeProvider,
				startedAtMs: time.Now().UnixMilli(),
			}
			activeTools[streamEvent.ItemID] = tool

			// Send tool call timeline event
			tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)

			// Update status
			oc.emitGenerationStatus(ctx, portal, state, "tool_use", "Searching the web...", &GenerationDetails{
				CurrentTool: "web_search",
				CallID:      tool.callID,
			})

			// Emit tool progress
			oc.emitToolProgress(ctx, portal, state, tool, ToolStatusRunning, "Searching...", 0)

		case "response.web_search_call.completed":
			touchTyping()
			// Web search completed
			tool, exists := activeTools[streamEvent.ItemID]
			if exists {
				// Send tool result timeline event
				resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, "", ResultStatusSuccess)

				// Track tool call
				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        tool.callID,
					ToolName:      "web_search",
					ToolType:      string(tool.toolType),
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(ResultStatusSuccess),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
					CallEventID:   tool.eventID.String(),
					ResultEventID: resultEventID.String(),
				})

				// Emit completion
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, "", map[string]any{
					"call_id":   tool.callID,
					"tool_name": "web_search",
					"status":    "completed",
				})
			}

			// Update status back to generating
			oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)

		case "response.image_generation_call.in_progress", "response.image_generation_call.generating":
			touchTyping()
			// Image generation in progress
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				tool = &activeToolCall{
					callID:      NewCallID(),
					toolName:    "image_generation",
					toolType:    ToolTypeProvider,
					startedAtMs: time.Now().UnixMilli(),
				}
				activeTools[streamEvent.ItemID] = tool

				// Send tool call timeline event
				tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)
			}

			// Update status
			oc.emitGenerationStatus(ctx, portal, state, "image_generating", "Generating image...", &GenerationDetails{
				CurrentTool: "image_generation",
				CallID:      tool.callID,
			})

			// Emit tool progress
			oc.emitToolProgress(ctx, portal, state, tool, ToolStatusRunning, "Generating image...", 50)

			log.Debug().Str("item_id", streamEvent.ItemID).Msg("Image generation in progress")

		case "response.image_generation_call.completed":
			touchTyping()
			// Image generation completed - the actual image data will be in response.completed
			tool, exists := activeTools[streamEvent.ItemID]
			if exists {
				// Track tool call
				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        tool.callID,
					ToolName:      "image_generation",
					ToolType:      string(tool.toolType),
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(ResultStatusSuccess),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
					CallEventID:   tool.eventID.String(),
				})

				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, "", map[string]any{
					"call_id":   tool.callID,
					"tool_name": "image_generation",
					"status":    "completed",
				})
			}
			log.Info().Str("item_id", streamEvent.ItemID).Msg("Image generation completed")

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

			// Emit final status
			oc.emitGenerationStatus(ctx, portal, state, "finalizing", "Completing response...", nil)

			log.Debug().Str("reason", state.finishReason).Str("response_id", state.responseID).Int("images", len(state.pendingImages)).Msg("Response stream completed")

		case "error":
			log.Error().Str("error", streamEvent.Message).Msg("Responses API stream error")
			// Check for context length error
			if strings.Contains(streamEvent.Message, "context_length") || strings.Contains(streamEvent.Message, "token") {
				return false, &ContextLengthError{
					OriginalError: fmt.Errorf("%s", streamEvent.Message),
				}, nil
			}
			apiErr := fmt.Errorf("API error: %s", streamEvent.Message)
			if state.initialEventID != "" {
				return false, nil, &NonFallbackError{Err: apiErr}
			}
			return false, nil, &PreDeltaError{Err: apiErr}
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		log.Error().Err(err).Msg("Responses API streaming error")
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
			log.Error().Msg("Failed to create continuation streaming request")
			break
		}

		// Process continuation stream events
		for stream.Next() {
			streamEvent := stream.Current()

			switch streamEvent.Type {
			case "response.output_text.delta":
				touchTyping()
				state.accumulated.WriteString(streamEvent.Delta)
				if !state.firstToken && state.initialEventID != "" {
					state.sequenceNum++
					oc.emitStreamDelta(ctx, portal, state, StreamContentText, streamEvent.Delta, nil)
				}

			case "response.reasoning_text.delta":
				touchTyping()
				state.reasoning.WriteString(streamEvent.Delta)
				if !state.firstToken && state.initialEventID != "" {
					state.sequenceNum++
					oc.emitStreamDelta(ctx, portal, state, StreamContentReasoning, streamEvent.Delta, nil)
				}

			case "response.function_call_arguments.delta":
				touchTyping()
				tool, exists := activeTools[streamEvent.ItemID]
				if !exists {
					if !ensureInitialEvent() {
						return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message for tool call")}
					}
					tool = &activeToolCall{
						callID:      NewCallID(),
						toolName:    streamEvent.Name,
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					activeTools[streamEvent.ItemID] = tool
					tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)
					oc.emitGenerationStatus(ctx, portal, state, "tool_use", fmt.Sprintf("Calling %s...", streamEvent.Name), &GenerationDetails{
						CurrentTool: streamEvent.Name,
						CallID:      tool.callID,
					})
				}
				tool.input.WriteString(streamEvent.Delta)

			case "response.function_call_arguments.done":
				touchTyping()
				tool, exists := activeTools[streamEvent.ItemID]
				if !exists {
					if !ensureInitialEvent() {
						return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message for tool call")}
					}
					tool = &activeToolCall{
						callID:      NewCallID(),
						toolName:    streamEvent.Name,
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					tool.input.WriteString(streamEvent.Arguments)
					activeTools[streamEvent.ItemID] = tool
				}

				// Update tool name from done event (delta events don't have the name)
				if tool.toolName == "" && streamEvent.Name != "" {
					tool.toolName = streamEvent.Name
				}

				tool.itemID = streamEvent.ItemID

				if !ensureInitialEvent() {
					return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message for tool call")}
				}

				resultStatus := ResultStatusSuccess
				var result string
				if !oc.isToolEnabled(meta, streamEvent.Name) {
					resultStatus = ResultStatusError
					result = fmt.Sprintf("Error: tool %s is disabled", streamEvent.Name)
				} else {
					toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
						Client:        oc,
						Portal:        portal,
						Meta:          meta,
						SourceEventID: state.sourceEventID,
					})
					var err error
					result, err = oc.executeBuiltinTool(toolCtx, portal, streamEvent.Name, streamEvent.Arguments)
					if err != nil {
						log.Warn().Err(err).Str("tool", streamEvent.Name).Msg("Tool execution failed (continuation)")
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
				args := strings.TrimSpace(tool.input.String())
				if args == "" {
					args = strings.TrimSpace(streamEvent.Arguments)
				}
				if args == "" {
					args = "{}"
				}
				name := strings.TrimSpace(tool.toolName)
				if name == "" {
					name = strings.TrimSpace(streamEvent.Name)
				}
				state.pendingFunctionOutputs = append(state.pendingFunctionOutputs, functionCallOutput{
					callID:    streamEvent.ItemID,
					name:      name,
					arguments: args,
					output:    result,
				})

				var inputMap map[string]any
				_ = json.Unmarshal([]byte(streamEvent.Arguments), &inputMap)

				resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, result, resultStatus)

				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        tool.callID,
					ToolName:      streamEvent.Name,
					ToolType:      string(tool.toolType),
					Input:         inputMap,
					Output:        map[string]any{"result": result},
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(resultStatus),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
					CallEventID:   tool.eventID.String(),
					ResultEventID: resultEventID.String(),
				})

				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, result, map[string]any{
					"call_id":   tool.callID,
					"tool_name": streamEvent.Name,
					"status":    "completed",
				})
				oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)

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
				log.Debug().Str("reason", state.finishReason).Str("response_id", state.responseID).Msg("Continuation stream completed")

			case "error":
				log.Error().Str("error", streamEvent.Message).Msg("Continuation stream error")
			}
		}

		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Continuation streaming error")
			break
		}
	}

	typingCtrl.MarkRunComplete()

	// Send final edit to persist complete content with metadata (including reasoning)
	if state.initialEventID != "" {
		oc.sendFinalAssistantTurn(ctx, portal, state, meta)
		oc.saveAssistantMessage(ctx, log, portal, state, meta)
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

	// Ensure model ghost is in the room before any operations
	if err := oc.ensureModelInRoom(ctx, portal); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure model is in room")
		// Continue anyway - typing will fail gracefully
	}

	// Create typing controller with TTL and automatic refresh
	typingCtrl := NewTypingController(oc, ctx, portal)
	typingCtrl.Start()
	defer typingCtrl.Stop()
	touchTyping := func() {
		typingCtrl.RefreshTTL()
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
		log.Error().Msg("Failed to create Chat Completions streaming request")
		return false, nil, &PreDeltaError{Err: fmt.Errorf("chat completions streaming not available")}
	}

	// Initialize streaming state with source event ID for [[reply_to_current]] support
	var sourceEventID id.EventID
	if evt != nil {
		sourceEventID = evt.ID
	}
	state := newStreamingState(meta, sourceEventID)

	// Track active tool calls by index
	activeTools := make(map[int]*activeToolCall)

	oc.emitGenerationStatus(ctx, portal, state, "starting", "Starting generation...", nil)

	for stream.Next() {
		chunk := stream.Current()

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
					oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
					state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID, state.sourceEventID)
					if state.initialEventID == "" {
						log.Error().Msg("Failed to send initial streaming message")
						return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message")}
					}
					// Initial message already includes this delta
					state.skipNextTextDelta = true
					oc.emitGenerationStatus(ctx, portal, state, "generating", "Generating response...", nil)
				}

				if !state.firstToken && state.initialEventID != "" {
					if state.skipNextTextDelta {
						state.skipNextTextDelta = false
						continue
					}
					state.sequenceNum++
					oc.emitStreamDelta(ctx, portal, state, StreamContentText, choice.Delta.Content, nil)
				}
			}

			// Handle tool calls from Chat Completions API
			for _, toolDelta := range choice.Delta.ToolCalls {
				touchTyping()
				toolIdx := int(toolDelta.Index)
				tool, exists := activeTools[toolIdx]
				if !exists {
					tool = &activeToolCall{
						callID:      NewCallID(),
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
					tool.toolName = toolDelta.Function.Name

					// Ensure we have an initial message for tool call events
					if state.firstToken {
						state.firstToken = false
						state.firstTokenAtMs = time.Now().UnixMilli()
						oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
						state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
						if state.initialEventID == "" {
							log.Error().Msg("Failed to send initial streaming message for tool call")
							return false, nil, &PreDeltaError{Err: fmt.Errorf("failed to send initial streaming message for tool call")}
						}
					}

					// Send tool call timeline event on first delta with name
					if tool.eventID == "" {
						tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)
						oc.emitGenerationStatus(ctx, portal, state, "tool_use",
							fmt.Sprintf("Calling %s...", tool.toolName), &GenerationDetails{
								CurrentTool: tool.toolName,
								CallID:      tool.callID,
							})
					}
				}

				// Accumulate arguments
				if toolDelta.Function.Arguments != "" {
					tool.input.WriteString(toolDelta.Function.Arguments)

					// Stream tool input delta
					if state.initialEventID != "" {
						state.sequenceNum++
						oc.emitStreamDelta(ctx, portal, state, StreamContentToolInput,
							toolDelta.Function.Arguments, map[string]any{
								"call_id":   tool.callID,
								"tool_name": tool.toolName,
							})
					}
				}
			}

			if choice.FinishReason != "" {
				state.finishReason = string(choice.FinishReason)
			}
		}
	}

	if err := stream.Err(); err != nil {
		if cle := ParseContextLengthError(err); cle != nil {
			return false, cle, nil
		}
		log.Error().Err(err).Msg("Chat Completions stream error")
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

			// Send tool result timeline event
			resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, result, resultStatus)

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
				CallEventID:   tool.eventID.String(),
				ResultEventID: resultEventID.String(),
			})

			// Stream result
			if state.initialEventID != "" {
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, result, map[string]any{
					"call_id":   tool.callID,
					"tool_name": tool.toolName,
					"status":    "completed",
				})
			}

			// Update status back to generating if there might be more content
			oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)
		}
	}

	typingCtrl.MarkRunComplete()

	state.completedAtMs = time.Now().UnixMilli()
	oc.emitGenerationStatus(ctx, portal, state, "completed", "Generation complete", nil)

	// Send final edit and save to database
	if state.initialEventID != "" {
		oc.sendFinalAssistantTurn(ctx, portal, state, meta)
		oc.saveAssistantMessage(ctx, log, portal, state, meta)
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
